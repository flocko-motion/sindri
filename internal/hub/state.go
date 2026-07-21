// package: hub / state
// type:    logic (the single read surface + change notifications)
// job:     assemble the whole board the UIs render — agents across every project
//
//	with live workflow state, merge-intents, and orphaned runtime, plus the
//	tasks of the selected project — and a tiny pub/sub so clients live-update
//	over /events. The central store is the read model; this is its projection.
//
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

// probeTimeout bounds each podman probe during a board read. A container that
// can't answer within this window is reported "down" rather than stalling the
// whole read — the board must stay responsive even when a pod is wedged.
const probeTimeout = 3 * time.Second

// statsTimeout bounds a single `stats` sample, which is slower than a liveness
// probe (the runtime samples over a short window before returning).
const statsTimeout = 8 * time.Second

// AgentView is an agent as the UIs see it: identity + live workflow + runtime.
// Status collapses runtime + workflow into one word: down | idle | working |
// submitted. Project (repoTag) and Repo (human path) tag which repo it belongs to,
// so the global Agents tab can show — and color — rows by repo.
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

// BoardState is the whole board in one payload. Agents and PRs are global (across
// every project, each row tagged with its repo); Tasks are the selected project's
// (td is per-repo, so a merged backlog would mislead); Projects is every repo the
// hub knows, for the TUI's repo switcher and repo labels.
type BoardState struct {
	Agents   []AgentView     `json:"agents"`
	Tasks    []store.Task    `json:"tasks"`
	PRs      []store.PR      `json:"prs"`
	Projects []store.Project `json:"projects"`
	Orphans  []string        `json:"orphans"` // pods with no roster entry (D14)
	Chat     ChatView        `json:"chat"`    // the user's chatroom: members + transcript
}

// State assembles the board: agents and PRs across all projects, tasks for the
// selected project (empty tag = none selected → no tasks).
func (h *Hub) State(selected string) (BoardState, error) {
	agentsRow, err := h.store.AllAgents()
	if err != nil {
		return BoardState{}, err
	}
	prs, err := h.store.AllPRs()
	if err != nil {
		return BoardState{}, err
	}
	// Only registered repos surface in the global views. A forgotten repo's PRs stay
	// in the db (keyed by its stable tag, so re-adding the repo reactivates them) but
	// drop out of the fleet PR tab — forgetting a repo means giving up its management,
	// not surfacing its records. (Its agents are already deleted, so AllAgents is clean.)
	registered := map[string]bool{}
	for _, p := range h.projects.Known() {
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

	// Liveness needs a podman round-trip per agent (inspect + a tmux exec); probe
	// every agent concurrently and time-bounded, so one wedged pod slows the read
	// by at most probeTimeout instead of serialising all of them. The same
	// list-clients probe that confirms the session is up also yields the dial-in
	// count, so the board shows who's attached at no extra cost.
	var wg sync.WaitGroup
	running := make([]bool, len(agentsRow))
	clients := make([]int, len(agentsRow))
	runtimes := make([]string, len(agentsRow)) // Claude's live runtime: busy|blocked|idle|""
	for i, a := range agentsRow {
		wg.Add(1)
		go func(i int, a store.Agent) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
			defer cancel()
			if !container.RunningContext(ctx, h.container(a.Project, a.Name)) {
				return
			}
			if cs, ok := h.agents.ClientsCtx(ctx, a.Project, a.Name); ok {
				running[i] = true
				clients[i] = len(cs)
				runtimes[i] = h.agents.RuntimeState(ctx, a.Project, a.Name) // what Claude is doing now
			}
		}(i, a)
	}
	wg.Wait()

	known := map[string]bool{}
	agents := make([]AgentView, 0, len(agentsRow))
	for i, a := range agentsRow {
		container := h.container(a.Project, a.Name)
		known[container] = true
		ps := h.store.For(a.Project)
		st, _ := ps.GetState(a.Name)
		// PR is the agent's own submitted PR (worker); a reviewer authors none, so
		// fall back to the PR it's currently reviewing — that's what the board should
		// show it working on.
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

	// Orphans: sindri pods with no roster entry, across every known project. One
	// podman ps per project, run concurrently and bounded like the liveness probes.
	projects := h.projects.Known()
	orphanLists := make([][]string, len(projects))
	for i, proj := range projects {
		wg.Add(1)
		go func(i int, proj store.Project) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
			defer cancel()
			if pods, err := container.ListByLabelContext(ctx, "sindri.project", proj.Path); err == nil {
				orphanLists[i] = pods
			}
		}(i, proj)
	}
	wg.Wait()

	var orphans []string
	for _, pods := range orphanLists {
		for _, p := range pods {
			if !known[p] {
				orphans = append(orphans, p)
			}
		}
	}
	chat, err := h.chatView()
	if err != nil {
		return BoardState{}, err
	}
	return BoardState{Agents: agents, Tasks: tasks, PRs: prs, Projects: projects, Orphans: orphans, Chat: chat}, nil
}

// AgentStatsView is one agent's resource snapshot for `agent stats`. Err is set
// (not swallowed) when the sample couldn't be read, so the row shows why instead
// of a misleading zero.
type AgentStatsView struct {
	Name          string `json:"name"`
	Repo          string `json:"repo"`
	MemUsageBytes int64  `json:"memUsageBytes"`
	MemLimitBytes int64  `json:"memLimitBytes"`
	Err           string `json:"err,omitempty"`
}

// StatsReport is the `agent stats` payload: which runtime is wired, plus a memory
// snapshot per running agent. Engine is included so the numbers are read in the
// right context (podman shares one VM; apple container is one micro-VM per agent).
type StatsReport struct {
	Engine string           `json:"engine"`
	Agents []AgentStatsView `json:"agents"`
}

// Stats returns the engine name and a resource snapshot for every running agent.
func (h *Hub) Stats() (StatsReport, error) {
	views, err := h.AllStats()
	return StatsReport{Engine: container.Name(), Agents: views}, err
}

// AllStats returns a resource snapshot for every RUNNING agent, gathered
// concurrently — each `stats` sample is slow (the runtime samples over a window),
// so serial would be N×that. Down agents are omitted (no VM to sample). A per-agent
// stats failure is reported in that row's Err, never silently dropped.
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


// projectPath resolves a project tag to its path, logging loudly on a real store
// error (distinct from an unknown project) instead of swallowing it into "". The
// string-returning callers (projectRoot/repoName/container) can't thread an error,
// so this is where the DB failure is surfaced.
func (h *Hub) projectPath(project string) (string, bool) {
	path, ok, err := h.store.ProjectPath(project)
	if err != nil {
		log.Printf("hub: resolve project path for %q failed: %v", project, err)
	}
	return path, ok
}

// repoName is a project's short human label (its directory name), resolved from the
// registry; falls back to the tag when the path is unknown.
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

// overlayRuntime folds Claude's live runtime (working|blocked|idle|"") into the
// workflow status, three states in herdr's own vocabulary: "blocked" = needs your
// attention now (any phase); "working" = busy; "idle" = not doing anything (no task,
// stalled at the prompt, or dropped to the maintenance shell). The live state
// replaces a plain working/idle phase; the meaningful workflow phases
// (submitted/collab/resolving/reviewing/planning) are kept, unless Claude is blocked.
// runtime "" (probe failed) leaves the phase untouched.
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

// Refresh re-syncs the selected project's tasks and notifies watchers. It's the
// [r]efresh hotkey / explicit user refresh, so it forces the GitHub scan past its
// TTL — the one place we want fresh issues on demand.
func (h *Hub) Refresh(project string) error {
	err := h.wf.ForceSyncTasks(project)
	h.notify()
	return err
}

// Log returns an agent's recent activity-log entries (oldest-first).
func (h *Hub) Log(project, name string) ([]store.Event, error) {
	return h.store.For(project).Events(name, 50)
}

// openPRFor returns the id of an agent's not-yet-merged PR in its project, if any.
func openPRFor(prs []store.PR, project, agent string) string {
	for _, p := range prs {
		if p.Project == project && p.Agent == agent && p.Status != "merged" {
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

// subscribe returns a buffered channel that ticks on every notify, plus an
// unsubscribe func.
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

// publish wakes every subscriber (non-blocking; a full buffer already means
// "refresh pending").
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
