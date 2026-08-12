// package: hub / state
// type:    logic (the single read surface + change notifications)
// job:     assemble the whole board the UIs render — agents across every project
// with live workflow state, merge-intents, and orphaned runtime, plus the
// tasks of the selected project — and a tiny pub/sub so clients live-update
// over /events. The central store is the read model; this is its projection.
// limits:  read-only assembly + notify; mutations live in their own methods.
package hub

import (
	"context"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/store"
)

// probeTimeout bounds each podman probe; a container that can't answer is "down", not a stalled read.
const probeTimeout = 3 * time.Second

// statsTimeout bounds one `stats` sample, slower than a probe (the runtime samples over a window).
const statsTimeout = 8 * time.Second

// AgentView is an agent as the UIs see it; it crosses the wire, so it is
// internal/api.AgentView under the name every existing caller here already uses.
type AgentView = api.AgentView

// BoardState is the whole board; it crosses the wire, so it is internal/api.BoardState
// under the name every existing caller here already uses.
type BoardState = api.BoardState

// State assembles the board; an empty selected tag means no project is chosen, so no tasks.
func (h *Hub) State(selected string) (BoardState, error) {
	agentsRow, err := h.store.AllAgents()
	if err != nil {
		return BoardState{}, err
	}
	prs, err := h.store.AllPRs()
	if err != nil {
		return BoardState{}, err
	}
	// Only registered repos surface in the global views: a forgotten repo's PRs stay in the db (keyed
	// by its stable tag, so re-adding reactivates them) but drop off the fleet tab. Read the registry
	// ONCE — twice doubled load on the single store connection and let one snapshot disagree with itself.
	projects, err := h.projects.Known()
	if err != nil {
		return BoardState{}, err // never render "no repos" from an unreadable registry
	}
	registered := map[string]bool{}
	for _, p := range projects {
		registered[p.Tag] = true
	}
	kept := prs[:0]
	for _, pr := range prs {
		if registered[pr.Project] {
			kept = append(kept, pr)
		}
	}
	prs = kept
	h.fillReviewers(prs)
	var tasks []store.Task
	var specMissing bool
	if selected != "" {
		if tasks, err = h.store.For(selected).AllTasks(); err != nil {
			return BoardState{}, err
		}
		root := h.projectRoot(selected)
		specMissing = h.wf.TaskSourceToolMissing(root)
	}

	// Liveness comes from the watchdog's last observation — a board read REPORTS it, never takes one.
	// Probing per request scaled cost with readers (overlapping polls, SSE, post-mutation refetches);
	// probes then lost their deadline and rendered as "down", flickering healthy agents (-> watchdog.go).
	running := make([]bool, len(agentsRow))
	clients := make([]int, len(agentsRow))
	runtimes := make([]string, len(agentsRow)) // Claude's live runtime: busy|blocked|idle|""
	// observed is carried separately because the zero value of running is a CLAIM: an agent
	// registered since the last sweep has been looked at by nothing, and reading its absent
	// observation as "not running" is the same error as trusting a stale listing.
	observed := make([]bool, len(agentsRow))
	for i, a := range agentsRow {
		if l, ok := h.watch.get(a.Project, a.Name); ok {
			running[i], clients[i], runtimes[i], observed[i] = l.up, l.clients, l.runtime, true
		}
	}

	// The orphan scan needs the pod list, not per-agent liveness; cached, so it reuses the watchdog's.
	podCtx, podCancel := context.WithTimeout(context.Background(), probeTimeout)
	existing, _ := container.ListByLabelCached(podCtx, "sindri.project", "")
	podCancel()

	known := map[string]bool{}
	agents := make([]AgentView, 0, len(agentsRow))
	for i, a := range agentsRow {
		container := h.container(a.Project, a.Name)
		known[container] = true
		ps := h.store.For(a.Project)
		st, _ := ps.GetState(a.Name)
		// A reviewer authors no PR, so fall back to the one it's reviewing — that's what it works on.
		pr := openPRFor(prs, a.Project, a.Name)
		if pr == "" {
			pr, _ = ps.ReviewingPR(a.Name)
		}
		holds := st.Task != "" || st.Container != "" || pr != ""
		status := overlayRuntime(h.agents.AgentStatus(a.Project, a.Name, running[i], observed[i], st.Phase), runtimes[i], holds)
		// A stall reads as plain "idle" otherwise, which is what let one hold a task unnoticed.
		if _, stalled := h.stalledFor(a.Project, a.Name, st.Phase, st.Container); stalled {
			status = "stalled"
		}
		tokens, window, _ := h.agents.ContextUsage(a.Project, a.Name)
		status = overlayFullness(status, h.wf.ContextFull(a.Project, a.Name), st.Task, st.Container, pr)
		agents = append(agents, AgentView{
			Project: a.Project, Repo: h.repoName(a.Project), Name: a.Name, Role: a.Role,
			Status:  status,
			Runtime: runtimes[i],
			Task:    st.Task, Feature: st.Container, Branch: st.Branch, PR: pr, Workspace: a.Workspace,
			Clients: clients[i], Container: container, Memory: a.Memory, Retired: a.Retired,
			ContextTokens: tokens, ContextWindow: window,
		})
	}

	// Orphans: sindri pods with no roster entry, from the listing the liveness probe already took.
	var orphans []string
	for _, p := range existing {
		if !known[p] {
			orphans = append(orphans, p)
		}
	}
	chat, err := h.chatView()
	if err != nil {
		return BoardState{}, err
	}
	// Carried in the snapshot so the TUI's recommendation matches the one hub startup prints.
	docs := make(map[string]RepoDocState, len(projects))
	for _, p := range projects {
		docs[p.Tag] = h.repoDocState(p.Path)
	}
	return BoardState{
		RuntimeHint: h.watch.runtimeHint(),
		Agents:      agents, Tasks: tasks, PRs: prs, Projects: projects, Orphans: orphans, Chat: chat,
		RepoDocs: docs, SpecCLIMissing: specMissing, StartedAt: h.startedAt.UTC().Format(time.RFC3339),
		DefaultMemory: agent.MemoryOrDefault(""),
	}, nil
}

// fillReviewers stamps each PR with the agent holding an open review of it. Whether a PR is being
// looked at, and by whom, is the hub's answer: a front-end deriving it from the reviews would be
// deciding rather than rendering, and the two would drift the first time the rule moved.
func (h *Hub) fillReviewers(prs []api.PR) {
	byProject := map[string]map[string]string{}
	for i, pr := range prs {
		active, ok := byProject[pr.Project]
		if !ok {
			var err error
			if active, err = h.store.For(pr.Project).ActiveReviewers(); err != nil {
				log.Printf("hub: active reviewers for %s: %v", pr.Project, err)
				active = map[string]string{}
			}
			byProject[pr.Project] = active
		}
		prs[i].Reviewer = active[pr.ID]
	}
}

// AgentStatsView is one agent's resource snapshot; it crosses the wire, so it is
// internal/api.AgentStatsView under the name every existing caller here already uses.
type AgentStatsView = api.AgentStatsView

// StatsReport is the `agent stats` payload; it crosses the wire, so it is
// internal/api.StatsReport under the name every existing caller here already uses.
type StatsReport = api.StatsReport

// Stats returns the engine name and a resource snapshot for every running agent.
func (h *Hub) Stats() (StatsReport, error) {
	views, err := h.AllStats()
	return StatsReport{Engine: container.Name(), Agents: views}, err
}

// AllStats snapshots every RUNNING agent concurrently — each sample is slow, so serial would be N×that.
// Down agents are omitted; a per-agent failure lands in that row's Err rather than being dropped.
func (h *Hub) AllStats() ([]AgentStatsView, error) {
	agentsRow, err := h.store.AllAgents()
	if err != nil {
		return nil, err
	}
	views := make([]AgentStatsView, len(agentsRow))
	var wg sync.WaitGroup
	for i, a := range agentsRow {
		wg.Add(1)
		go func(i int, a store.Agent) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), statsTimeout)
			defer cancel()
			c := h.container(a.Project, a.Name)
			if !container.RunningContext(ctx, c) {
				return // down — no VM to sample; filtered out below (Name stays "")
			}
			v := AgentStatsView{Name: a.Name, Repo: h.repoName(a.Project)}
			if s, serr := container.Stats(ctx, c); serr != nil {
				v.Err = serr.Error()
			} else {
				v.MemUsageBytes, v.MemLimitBytes = s.MemoryUsageBytes, s.MemoryLimitBytes
			}
			views[i] = v
		}(i, a)
	}
	wg.Wait()

	out := make([]AgentStatsView, 0, len(views))
	for _, v := range views {
		if v.Name != "" { // running agents only
			out = append(out, v)
		}
	}
	return out, nil
}

// projectPath resolves a project tag to its path, logging loudly on a real store error (as opposed to
// an unknown project) — the string-returning callers can't thread one, so it must surface here.
func (h *Hub) projectPath(project string) (string, bool) {
	path, ok, err := h.store.ProjectPath(project)
	if err != nil {
		log.Printf("hub: resolve project path for %q failed: %v", project, err)
	}
	return path, ok
}

// repoName is a project's directory name from the registry, falling back to the tag.
func (h *Hub) repoName(project string) string {
	if path, ok := h.projectPath(project); ok {
		return filepath.Base(path)
	}
	return project
}

// container is the podman container name for an agent, resolved via the registry.
func (h *Hub) container(project, name string) string {
	root, _ := h.projectPath(project)
	return Container(root, name)
}

// overlayRuntime folds Claude's live runtime into the workflow status: "signed-out" = unreachable
// until a human acts, "blocked" = needs you now (any phase), "working" = busy, "idle" = nothing
// doing. It replaces a plain working/idle phase but keeps the meaningful ones; runtime "" (probe
// failed) changes nothing. holds says whether the agent has work in hand.
func overlayRuntime(status, runtime string, holds bool) string {
	switch runtime {
	case "signed-out":
		// Outranks every phase: whatever was asked of it, nothing is happening and nothing can reach it.
		return "signed-out"
	case "blocked":
		return "blocked"
	case "working", "idle":
		// A pane in motion is not work in hand. An agent reading a broadcast, or answering the user,
		// moves its screen while holding nothing — and "working" is a claim about the workflow, so
		// against an empty task column it states something that cannot be true.
		if runtime == "working" && !holds {
			return status
		}
		if status == "working" || status == "idle" {
			return runtime
		}
	}
	return status
}

// overlayFullness shows "full" only where it EXPLAINS something: an agent holding nothing, which
// claimNext is passing over for exactly this reason. Fullness is not an activity, so anywhere else
// it would replace the one fact the column exists to carry — and the fill is on the board as
// ContextTokens for anyone who wants the number.
//
// Held work is checked directly rather than trusted to the word: a quiet runtime probe reads a
// task-holder as "idle" (-> overlayRuntime) before it has been still long enough to say "stalled",
// and "full" on an agent mid-task invites clearing a context the hub refuses to clear anyway.
func overlayFullness(status string, full bool, task, feature, pr string) string {
	if full && status == "idle" && task == "" && feature == "" && pr == "" {
		return "full"
	}
	return status
}

// Refresh re-syncs tasks and notifies watchers; being the user's explicit refresh it forces the
// GitHub scan past its TTL.
func (h *Hub) Refresh(project string) error {
	err := h.wf.ForceSyncTasks(project)
	h.notify()
	return err
}

// Log returns an agent's recent activity-log entries (oldest-first).
func (h *Hub) Log(project, name string) ([]store.Event, error) {
	return h.store.For(project).Events(name, 50)
}

// openPRFor returns the id of an agent's still-open PR in its project, if any. Open-ness is PROpen's
// to define, the same rule the PRs tab lists by — deciding it here instead left a scrapped PR
// attributed to its author on the Agents tab while the PRs tab, correctly, showed nothing.
func openPRFor(prs []store.PR, project, agent string) string {
	for _, p := range prs {
		if p.Project == project && p.Agent == agent && PROpen(p) {
			return p.ID
		}
	}
	return ""
}

// --- change notifications (pub/sub for /events) ---

type bus struct {
	mu   sync.Mutex
	subs map[chan struct{}]bool
}

func newBus() *bus { return &bus{subs: map[chan struct{}]bool{}} }

// subscribe returns a buffered channel that ticks on every notify, plus an unsubscribe func.
func (b *bus) subscribe() (chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	b.subs[ch] = true
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		close(ch)
		b.mu.Unlock()
	}
}

// publish wakes every subscriber (non-blocking; a full buffer already means "refresh pending").
func (b *bus) publish() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// notify signals that board state changed (called after every mutation).
func (h *Hub) notify() {
	if h.events != nil {
		h.events.publish()
	}
}
