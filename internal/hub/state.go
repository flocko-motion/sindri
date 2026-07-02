// package: hub / state
// type:    logic (the single read surface + change notifications)
// job:     assemble the whole board the UIs render — agents across every project
//          with live workflow state, merge-intents, and orphaned runtime, plus the
//          tasks of the selected project — and a tiny pub/sub so clients live-update
//          over /events. The central store is the read model; this is its projection.
// limits:  read-only assembly + notify; mutations live in their own methods.
package hub

import (
	"path/filepath"
	"sync"

	"github.com/flo-at/sindri/internal/adapter/pod"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/hub/store"
)

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
}

// BoardState is the whole board in one payload. Agents and PRs are global (across
// every project, each row tagged with its repo); Tasks are the selected project's
// (td is per-repo, so a merged backlog would mislead).
type BoardState struct {
	Agents  []AgentView  `json:"agents"`
	Tasks   []store.Task `json:"tasks"`
	PRs      []store.PR  `json:"prs"`
	Orphans []string     `json:"orphans"` // pods with no roster entry (D14)
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

	known := map[string]bool{}
	agents := make([]AgentView, 0, len(agentsRow))
	for _, a := range agentsRow {
		known[h.container(a.Project, a.Name)] = true
		st, _ := h.store.For(a.Project).GetState(a.Name)
		running := h.agentAlive(a.Project, a.Name)
		agents = append(agents, AgentView{
			Project: a.Project, Repo: h.repoName(a.Project), Name: a.Name, Role: a.Role,
			Status: h.agentStatus(a.Project, a.Name, running, st.Phase),
			Task:   st.Task, Branch: st.Branch, PR: openPRFor(prs, a.Project, a.Name), Workspace: a.Workspace,
		})
	}

	// Orphans: sindri pods with no roster entry, across every known project.
	var orphans []string
	for _, proj := range h.knownProjects() {
		if pods, err := pod.ListByLabel("sindri.project", proj.Path); err == nil {
			for _, p := range pods {
				if !known[p] {
					orphans = append(orphans, p)
				}
			}
		}
	}
	return BoardState{Agents: agents, Tasks: tasks, PRs: prs, Orphans: orphans}, nil
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

// agentAlive reports whether an agent is running (pod up and tmux session live).
func (h *Hub) agentAlive(project, name string) bool {
	return pod.Running(h.container(project, name)) && h.sessionAlive(project, name)
}

// sessionAlive reports whether the agent's tmux session is up inside its pod.
func (h *Hub) sessionAlive(project, name string) bool {
	_, err := pod.Exec(h.container(project, name), append([]string{"tmux"}, tmux.HasSession(name)...)...)
	return err == nil
}

// AgentPane returns the last `lines` rows of what the agent is showing — the live
// tmux screen once up, else the container's startup logs, else the captured launch
// output. Empty when truly down.
func (h *Hub) AgentPane(project, name string, lines int) (string, error) {
	if h.sessionAlive(project, name) {
		out, err := pod.Exec(h.container(project, name), append([]string{"tmux"}, tmux.CapturePane(name, lines)...)...)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	if logs := pod.Logs(h.container(project, name), lines); logs != "" {
		return logs, nil
	}
	return h.launchOutput(project, name), nil
}

// PodInfo returns a short summary of an agent's podman container for the Agents-tab
// pod view.
func (h *Hub) PodInfo(project, name string) (string, error) {
	c := h.container(project, name)
	header := "container: " + c + "\n\n"
	if info := pod.Info(c); info != "" {
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
