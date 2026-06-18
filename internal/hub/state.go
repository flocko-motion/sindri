// package: hub / state
// type:    logic (the single read surface + change notifications)
// job:     assemble the whole board the UIs render — agents with live workflow
//          state, open tasks, merge-intents, and orphaned runtime — and a tiny
//          pub/sub so clients can live-update over /events. hub.db is the read
//          model; this is its projection (D-hub: everything reads /state).
// limits:  read-only assembly + notify; mutations live in their own methods.
package hub

import (
	"sync"

	"github.com/flo-at/sindri/internal/adapter/pod"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/hub/store"
)

// AgentView is an agent as the UIs see it: identity + live workflow + runtime.
// Status collapses runtime + workflow into one word: down (not running) |
// idle | working | submitted.
type AgentView struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	Task      string `json:"task"`
	Branch    string `json:"branch"`
	PR        string `json:"pr"`
	Workspace string `json:"workspace"` // the agent's git worktree path (repo-relative)
}

// BoardState is the whole board in one payload — the single read surface.
type BoardState struct {
	Agents  []AgentView `json:"agents"`
	Tasks   []store.Task `json:"tasks"`
	PRs     []store.PR   `json:"prs"`
	Orphans []string     `json:"orphans"` // pods with no roster entry (D14)
}

// State assembles the board from hub.db plus live pod inspection.
func (h *Hub) State() (BoardState, error) {
	roster, err := h.store.Roster()
	if err != nil {
		return BoardState{}, err
	}
	prs, err := h.store.PRs()
	if err != nil {
		return BoardState{}, err
	}
	tasks, err := h.store.AllTasks() // all statuses; UIs filter open/closed/all
	if err != nil {
		return BoardState{}, err
	}

	known := map[string]bool{}
	agents := make([]AgentView, 0, len(roster))
	for _, a := range roster {
		known[Container(a.Name)] = true
		st, _ := h.store.GetState(a.Name)
		// One status word, reconciling transient intent (launching/stopping) with
		// observed runtime. "Alive" means the tmux session is up and attachable —
		// not merely that the pod (a sleep) exists.
		running := pod.Running(Container(a.Name)) && h.sessionAlive(a.Name)
		status := h.agentStatus(a.Name, running, st.Phase)
		agents = append(agents, AgentView{
			Name: a.Name, Role: a.Role, Status: status,
			Task: st.Task, Branch: st.Branch, PR: openPRFor(prs, a.Name), Workspace: a.Workspace,
		})
	}

	var orphans []string
	if pods, err := pod.ListByLabel("sindri.project", h.root); err == nil {
		for _, p := range pods {
			if !known[p] {
				orphans = append(orphans, p)
			}
		}
	}
	return BoardState{Agents: agents, Tasks: tasks, PRs: prs, Orphans: orphans}, nil
}

// sessionAlive reports whether the agent's tmux session is up inside its pod.
func (h *Hub) sessionAlive(name string) bool {
	// tmux.* builders return the subcommand only — the command is "tmux".
	_, err := pod.Exec(Container(name), append([]string{"tmux"}, tmux.HasSession(name)...)...)
	return err == nil
}

// AgentPane returns the last `lines` rows of what the agent is showing. Once its
// tmux session is up that's the live screen (capture-pane). While the pod is
// still booting (tmux not yet up) it falls back to the container's startup logs
// so a launch shows progress instead of silence. Empty when truly down.
func (h *Hub) AgentPane(name string, lines int) (string, error) {
	if h.sessionAlive(name) {
		// tmux.* builders return the subcommand only — the command is "tmux".
		out, err := pod.Exec(Container(name), append([]string{"tmux"}, tmux.CapturePane(name, lines)...)...)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	// tmux isn't up — show the container's logs whether it's still booting OR has
	// already exited, so a crash-on-boot is visible. If there's no container yet
	// (image still building), fall back to the captured launch output.
	if logs := pod.Logs(Container(name), lines); logs != "" {
		return logs, nil
	}
	return h.launchOutput(name), nil
}

// Refresh re-syncs tasks from the source of truth and notifies watchers (the
// user `refresh` action, D15).
func (h *Hub) Refresh() error {
	err := h.SyncTasks()
	h.notify()
	return err
}

// Log returns an agent's recent activity-log entries (the per-worker timeline,
// D12), oldest-first.
func (h *Hub) Log(name string) ([]store.Event, error) {
	return h.store.Events(name, 50)
}

// openPRFor returns the id of an agent's not-yet-merged PR, if any.
func openPRFor(prs []store.PR, agent string) string {
	for _, p := range prs {
		if p.Agent == agent && p.Status != "merged" {
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

// notify wakes every subscriber (non-blocking; a full buffer already means
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
