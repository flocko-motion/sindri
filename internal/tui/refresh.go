// package: tui / refresh
// type:    ui (live-update plumbing)
// job:     keep the board fresh between hub notifications — a periodic poll
//          while on the Agents tab, and an immediate post-mutation refetch so
//          actions (delete/role/new) reflect at once. All fetch via /state and
//          deliver polledMsg, which updates the board without re-arming the SSE
//          waiter.
package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/client"
)

// tickCmd fires a tickMsg every refreshInterval — the heartbeat behind the
// agents-tab auto-refresh.
func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
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

// logFetchCmd refetches an agent's activity log.
func logFetchCmd(cl *client.HTTP, agent string) tea.Cmd {
	return func() tea.Msg {
		evs, _ := cl.Log(agent)
		return logMsg{agent, evs}
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
	return tea.Batch(logFetchCmd(m.cl, id), paneFetchCmd(m.cl, id))
}

// mutateThenRefresh runs a hub mutation, then immediately fetches fresh state so
// the board reflects it without waiting for the SSE push or the next poll tick.
func mutateThenRefresh(cl *client.HTTP, mutate func()) tea.Cmd {
	return func() tea.Msg {
		if cl == nil {
			return nil
		}
		mutate()
		st, err := cl.State()
		if err != nil {
			return nil
		}
		return polledMsg(st)
	}
}
