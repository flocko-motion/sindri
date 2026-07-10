// package: tui / refresh
// type:    ui (live-update plumbing)
// job:     keep the board fresh between hub notifications — a periodic poll
//          while on the Agents tab, and an immediate post-mutation refetch so
//          actions (delete/role/new) reflect at once. All fetch via /state and
//          deliver polledMsg, which updates the board without re-arming the SSE
//          waiter.
// limits:  scheduling and fetch only; rendering the refreshed board is the tabs'
//          and the data is the hub's (-> client).
package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/hub"
)

// waitForState blocks on the /events channel for the next board snapshot, tagging it
// with the subscription generation so a snapshot from a stream abandoned by a repo
// switch can be ignored. A closed channel surfaces as a fatal errMsg.
func waitForState(ch <-chan hub.BoardState, gen int) tea.Cmd {
	return func() tea.Msg {
		st, ok := <-ch
		if !ok {
			return errMsg{err: fmt.Errorf("hub connection closed"), gen: gen}
		}
		return stateMsg{st: st, gen: gen}
	}
}

// tickCmd fires a tickMsg every refreshInterval — the heartbeat behind the
// agents-tab auto-refresh.
func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// refreshTaskCommentsCmd force-syncs one task's comments from its source (bypassing
// the TTL), then re-fetches its detail so the fresh thread shows at once.
func refreshTaskCommentsCmd(cl *client.HTTP, id string) tea.Cmd {
	return func() tea.Msg {
		_ = cl.RefreshTaskComments(id)
		t, err := cl.TaskInfo(id)
		if err != nil {
			return nil
		}
		return taskMsg{id, t}
	}
}

// chatHeartbeatCmd tells the hub the user is present in the chatroom (fired while
// the Chat tab is open). Presence keeps the room unlocked for agents.
func chatHeartbeatCmd(cl *client.HTTP) tea.Cmd {
	return func() tea.Msg {
		_ = cl.ChatHeartbeat()
		return nil
	}
}

// pollStateCmd fetches a fresh board snapshot (re-evaluating live agent state)
// without disturbing the SSE waiter.
func pollStateCmd(cl *client.HTTP) tea.Cmd {
	return func() tea.Msg {
		st, err := cl.State()
		if err != nil {
			return nil
		}
		return polledMsg(st)
	}
}

// paneFetchCmd captures the agent's live tmux screen.
func paneFetchCmd(cl *client.HTTP, agent string) tea.Cmd {
	return func() tea.Msg {
		out, _ := cl.AgentPane(agent, paneLines)
		return paneMsg{agent, out}
	}
}

// podFetchCmd fetches the agent's podman pod-info summary.
func podFetchCmd(cl *client.HTTP, agent string) tea.Cmd {
	return func() tea.Msg {
		out, _ := cl.PodInfo(agent)
		return agentPodMsg{agent, out}
	}
}

// logFetchCmd refetches an agent's activity log.
func logFetchCmd(cl *client.HTTP, agent string) tea.Cmd {
	return func() tea.Msg {
		evs, _ := cl.Log(agent)
		return logMsg{agent, evs}
	}
}

// clientsFetchCmd fetches the humans attached to an agent's tmux session, so the
// detail view shows the same dial-ins as the CLI's `agent info`.
func clientsFetchCmd(cl *client.HTTP, agent string) tea.Cmd {
	return func() tea.Msg {
		cs, _ := cl.Clients(agent)
		return clientsMsg{agent, cs}
	}
}

// agentLiveCmds refetches the selected agent's log + screen so they stay live on
// every board update while on the Agents tab (e.g. a freshly-logged launch).
func (m model) agentLiveCmds() tea.Cmd {
	if m.tab != 1 || m.cl == nil {
		return nil
	}
	id := m.selID()
	if id == "" {
		return nil
	}
	cmds := []tea.Cmd{logFetchCmd(m.cl, id), paneFetchCmd(m.cl, id), clientsFetchCmd(m.cl, id)}
	if m.agentView == "pod" { // keep the pod view live too
		cmds = append(cmds, podFetchCmd(m.cl, id))
	}
	return tea.Batch(cmds...)
}

// refreshCmd asks the hub to re-sync tasks from the source of truth.
func (m *model) refreshCmd() tea.Cmd {
	cl := m.cl
	if cl == nil {
		return nil
	}
	return func() tea.Msg { cl.Refresh(); return nil }
}

// action runs a mutating hub call for the current selection: surfaces failures
// in the error modal and refetches state on success.
func (m *model) action(fn func(id string) error) tea.Cmd {
	id := m.selID()
	if id == "" || m.cl == nil {
		return nil
	}
	cl := m.cl
	return func() tea.Msg {
		if err := fn(id); err != nil {
			return errModalMsg{err}
		}
		st, err := cl.State()
		if err != nil {
			return errModalMsg{err} // never swallow — the hub became unreachable
		}
		return polledMsg(st)
	}
}

// mutateThenRefresh runs a hub mutation, then immediately fetches fresh state so
// the board reflects it without waiting for the SSE push or the next poll tick.
// Both the mutation and the refresh surface their error in the modal — never
// swallowed.
func mutateThenRefresh(cl *client.HTTP, mutate func() error) tea.Cmd {
	return func() tea.Msg {
		if cl == nil {
			return nil
		}
		if err := mutate(); err != nil {
			return errModalMsg{err}
		}
		st, err := cl.State()
		if err != nil {
			return errModalMsg{err}
		}
		return polledMsg(st)
	}
}
