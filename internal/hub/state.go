// package: hub / state
// type:    logic (the single read surface + change notifications)
// job:     assemble the whole board the UIs render — agents across every project
//          with live workflow state, merge-intents, and orphaned runtime, plus the
//          tasks of the selected project — and a tiny pub/sub so clients live-update
//          over /events. The central store is the read model; this is its projection.
// limits:  read-only assembly + notify; mutations live in their own methods.
package hub

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/hub/store"
)

// probeTimeout bounds each podman probe during a board read. A container that
// can't answer within this window is reported "down" rather than stalling the
// whole read — the board must stay responsive even when a pod is wedged.
const probeTimeout = 3 * time.Second

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
}

// ClientView is one human attached to an agent's tmux session — a live dial-in.
// Surfaced so the UIs can show who's watching and whether they can type (a
// read-only client observes but can't send keys). An orphaned client (a dropped
// `podman exec` that left its tmux attach behind) shows up here too, which is how
// a session that "sees but can't type" becomes visible instead of mysterious.
type ClientView struct {
	TTY      string `json:"tty"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	ReadOnly bool   `json:"read_only"`
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
	for i, a := range agentsRow {
		wg.Add(1)
		go func(i int, a store.Agent) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
			defer cancel()
			if !container.RunningContext(ctx, h.container(a.Project, a.Name)) {
				return
			}
			if cs, ok := h.clientsCtx(ctx, a.Project, a.Name); ok {
				running[i] = true
				clients[i] = len(cs)
			}
		}(i, a)
	}
	wg.Wait()

	known := map[string]bool{}
	agents := make([]AgentView, 0, len(agentsRow))
	for i, a := range agentsRow {
		container := h.container(a.Project, a.Name)
		known[container] = true
		st, _ := h.store.For(a.Project).GetState(a.Name)
		agents = append(agents, AgentView{
			Project: a.Project, Repo: h.repoName(a.Project), Name: a.Name, Role: a.Role,
			Status: h.agentStatus(a.Project, a.Name, running[i], st.Phase),
			Task:   st.Task, Branch: st.Branch, PR: openPRFor(prs, a.Project, a.Name), Workspace: a.Workspace,
			Clients: clients[i], Container: container,
		})
	}

	// Orphans: sindri pods with no roster entry, across every known project. One
	// podman ps per project, run concurrently and bounded like the liveness probes.
	projects := h.knownProjects()
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
	return BoardState{Agents: agents, Tasks: tasks, PRs: prs, Projects: projects, Orphans: orphans}, nil
}

// knownProjects returns the registry's projects (best-effort; empty on error).
func (h *Hub) knownProjects() []store.Project {
	ps, _ := h.store.Projects()
	return ps
}

// repoName is a project's short human label (its directory name), resolved from the
// registry; falls back to the tag when the path is unknown.
func (h *Hub) repoName(project string) string {
	if path, ok := h.store.ProjectPath(project); ok {
		return filepath.Base(path)
	}
	return project
}

// container is the podman container name for an agent, resolved via the registry.
func (h *Hub) container(project, name string) string {
	root, _ := h.store.ProjectPath(project)
	return Container(root, name)
}

// launchDiagnostic reports WHY a just-launched agent isn't observed up, so a
// timeout is actionable instead of a shrug. It re-runs the two liveness probes
// through the runtime, capturing their errors: the running check, then the tmux
// session check inside the container. Whichever fails (and its error) is almost
// always the real cause — a runtime that can't answer, or a session that never
// started.
func (h *Hub) launchDiagnostic(project, name string) string {
	c := h.container(project, name)
	if !container.Running(c) {
		return fmt.Sprintf("the runtime does not report container %s as running [%s]", c,
			container.Diagnose(context.Background(), c))
	}
	if out, err := container.Exec(c, append([]string{"tmux"}, tmux.HasSession(name)...)...); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Sprintf("container is running but its tmux session check failed: %s", msg)
	}
	return "container and session both answer now — the liveness checks had been failing transiently"
}

// agentAlive reports whether an agent is running (pod up and tmux session live).
func (h *Hub) agentAlive(project, name string) bool {
	return h.agentAliveCtx(context.Background(), project, name)
}

// agentAliveCtx is agentAlive with each podman probe bounded by ctx, so a wedged
// pod times out to "down" instead of blocking. Used by the board read.
func (h *Hub) agentAliveCtx(ctx context.Context, project, name string) bool {
	return container.RunningContext(ctx, h.container(project, name)) && h.sessionAliveCtx(ctx, project, name)
}

// Clients lists the humans attached to an agent's tmux session (dial-ins). Errors
// when the agent isn't running. The headless read behind both `agent info` and the
// TUI detail view, so they show the same thing.
func (h *Hub) Clients(project, name string) ([]ClientView, error) {
	cs, ok := h.clientsCtx(context.Background(), project, name)
	if !ok {
		return nil, fmt.Errorf("agent %q is not running", name)
	}
	return cs, nil
}

// clientsCtx parses `tmux list-clients` for the agent's session, bounded by ctx.
// ok=false when the session is absent (so it also serves as a liveness probe).
func (h *Hub) clientsCtx(ctx context.Context, project, name string) (cs []ClientView, ok bool) {
	out, err := container.ExecContext(ctx, h.container(project, name), append([]string{"tmux"}, tmux.ListClients(name)...)...)
	if err != nil {
		return nil, false
	}
	return parseClients(string(out)), true
}

// parseClients turns list-clients output (one "tty width height readonly" line per
// client) into ClientViews. Malformed lines are skipped rather than failing the
// whole read.
func parseClients(out string) []ClientView {
	var cs []ClientView
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		w, _ := strconv.Atoi(f[1])
		ht, _ := strconv.Atoi(f[2])
		cs = append(cs, ClientView{TTY: f[0], Width: w, Height: ht, ReadOnly: f[3] == "1"})
	}
	return cs
}

// FormatClients renders attached clients for a human — shared by the CLI's
// `agent info` and the TUI detail view so both read identically. Empty when
// nobody's attached.
func FormatClients(cs []ClientView) string {
	if len(cs) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "clients:   %d attached\n", len(cs))
	for _, c := range cs {
		mode := "read-write"
		if c.ReadOnly {
			mode = "read-only"
		}
		fmt.Fprintf(&b, "  %s  %dx%d  %s\n", c.TTY, c.Width, c.Height, mode)
	}
	return b.String()
}

// sessionAlive reports whether the agent's tmux session is up inside its pod.
func (h *Hub) sessionAlive(project, name string) bool {
	return h.sessionAliveCtx(context.Background(), project, name)
}

// sessionAliveCtx is sessionAlive bounded by ctx.
func (h *Hub) sessionAliveCtx(ctx context.Context, project, name string) bool {
	_, err := container.ExecContext(ctx, h.container(project, name), append([]string{"tmux"}, tmux.HasSession(name)...)...)
	return err == nil
}

// AgentPane returns the last `lines` rows of what the agent is showing — the live
// tmux screen once up, else the container's startup logs, else the captured launch
// output. Empty when truly down.
func (h *Hub) AgentPane(project, name string, lines int) (string, error) {
	if h.sessionAlive(project, name) {
		out, err := container.Exec(h.container(project, name), append([]string{"tmux"}, tmux.CapturePane(name, lines)...)...)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	if logs := container.Logs(h.container(project, name), lines); logs != "" {
		return logs, nil
	}
	return h.launchOutput(project, name), nil
}

// PodInfo returns a short summary of an agent's podman container for the Agents-tab
// pod view.
func (h *Hub) PodInfo(project, name string) (string, error) {
	c := h.container(project, name)
	header := "container: " + c + "\n\n"
	if info := container.Info(c); info != "" {
		return header + info, nil
	}
	return header + "(no container — agent is down)", nil
}

// Refresh re-syncs the selected project's tasks and notifies watchers.
func (h *Hub) Refresh(project string) error {
	err := h.SyncTasks(project)
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
