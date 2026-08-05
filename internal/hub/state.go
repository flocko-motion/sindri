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

	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/store"
)

// probeTimeout bounds each podman probe; a container that can't answer is "down", not a stalled read.
const probeTimeout = 3 * time.Second

// statsTimeout bounds one `stats` sample, slower than a probe (the runtime samples over a window).
const statsTimeout = 8 * time.Second

// AgentView is an agent as the UIs see it; Status collapses runtime + workflow into one word:
// down | idle | working | submitted.
type AgentView struct {
	Project   string `json:"project"`
	Repo      string `json:"repo"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	Task      string `json:"task"`
	Branch    string `json:"branch"`
	PR        string `json:"pr"`
	Workspace string `json:"workspace"` // the agent's git worktree path (repo-relative)
	Clients   int    `json:"clients"`   // humans attached to its tmux session (dial-ins)
	Container string `json:"container"` // podman container name (project-resolved, so cross-repo callers target the right pod)
	Memory    string `json:"memory"`    // configured RAM limit ("" = hub default)
	Runtime   string `json:"runtime"`   // Claude's live runtime: "working"|"blocked"|"idle"|"" (folded into Status; kept raw for the herdr projection)
}

// BoardState is the whole board: Agents and PRs global, Tasks only the selected project's.
type BoardState struct {
	Agents   []AgentView             `json:"agents"`
	Tasks    []store.Task            `json:"tasks"`
	PRs      []store.PR              `json:"prs"`
	Projects []store.Project         `json:"projects"`
	Orphans  []string                `json:"orphans"`   // pods with no roster entry (D14)
	Chat     ChatView                `json:"chat"`      // the user's chatroom: members + transcript
	RepoDocs map[string]RepoDocState `json:"repo_docs"` // per repo tag: its architecture doc + any gap
}

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
	var tasks []store.Task
	if selected != "" {
		if tasks, err = h.store.For(selected).AllTasks(); err != nil {
			return BoardState{}, err
		}
	}

	// Liveness comes from the watchdog's last observation — a board read REPORTS it, never takes one.
	// Probing per request scaled cost with readers (overlapping polls, SSE, post-mutation refetches);
	// probes then lost their deadline and rendered as "down", flickering healthy agents (-> watchdog.go).
	running := make([]bool, len(agentsRow))
	clients := make([]int, len(agentsRow))
	runtimes := make([]string, len(agentsRow)) // Claude's live runtime: busy|blocked|idle|""
	for i, a := range agentsRow {
		if l, ok := h.watch.get(a.Project, a.Name); ok {
			running[i], clients[i], runtimes[i] = l.up, l.clients, l.runtime
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
		agents = append(agents, AgentView{
			Project: a.Project, Repo: h.repoName(a.Project), Name: a.Name, Role: a.Role,
			Status:  overlayRuntime(h.agents.AgentStatus(a.Project, a.Name, running[i], st.Phase), runtimes[i]),
			Runtime: runtimes[i],
			Task:    st.Task, Branch: st.Branch, PR: pr, Workspace: a.Workspace,
			Clients: clients[i], Container: container, Memory: a.Memory,
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
	return BoardState{Agents: agents, Tasks: tasks, PRs: prs, Projects: projects, Orphans: orphans, Chat: chat, RepoDocs: docs}, nil
}

// AgentStatsView is one agent's resource snapshot; Err is set, not swallowed into a misleading zero.
type AgentStatsView struct {
	Name          string `json:"name"`
	Repo          string `json:"repo"`
	MemUsageBytes int64  `json:"memUsageBytes"`
	MemLimitBytes int64  `json:"memLimitBytes"`
	Err           string `json:"err,omitempty"`
}

// StatsReport is the `agent stats` payload. Engine is included so the numbers are read in context:
// podman shares one VM, apple container is one micro-VM per agent.
type StatsReport struct {
	Engine string           `json:"engine"`
	Agents []AgentStatsView `json:"agents"`
}

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

// overlayRuntime folds Claude's live runtime into the workflow status: "blocked" = needs you now (any
// phase), "working" = busy, "idle" = nothing doing. It replaces a plain working/idle phase but keeps
// the meaningful ones; runtime "" (probe failed) changes nothing.
func overlayRuntime(status, runtime string) string {
	switch runtime {
	case "blocked":
		return "blocked"
	case "working", "idle":
		if status == "working" || status == "idle" {
			return runtime
		}
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
