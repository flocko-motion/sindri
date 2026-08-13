// package: tui / agent detail lines
// type:    ui (Agents tab — the detail as plain text)
// job:     an agent's detail rendered as plain lines, which is what the full-screen modal shows,
// what `Y` copies, and what other tabs borrow when they point at an agent. The
// interactive column is built from items instead (-> agentItems).
// limits:  rendering only; the fields come off the board.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/theme"
)

func (m model) agentDetailLines() []string {
	a, ok := m.selAgent()
	if !ok {
		return []string{dimStyle.Render("(orphan — no roster entry; '" + keyDelete + "' removes it)")}
	}
	return m.agentDetailFor(a)
}

// agentDetailFor renders an agent's detail; the activity log only for the selected one (lazy fetch).
func (m model) agentDetailFor(a api.AgentView) []string {
	// A reviewer's own Task is always "" — the task belongs to the agent that wrote the PR — so
	// fall back to what that PR is for, matching agentItems.
	taskID := a.Task
	if taskID == "" {
		taskID = m.prTask(a.PR)
	}
	ls := []string{
		"agent:     " + a.Name,
		"role:      " + a.Role,
		"status:    " + a.Status,
	}
	if a.Escalation != "" { // what it is waiting on, wherever this detail is read (modal, yank, other tabs)
		ls = append(ls, "escalated: "+a.Escalation)
	}
	ls = append(ls,
		"task:      "+m.taskLabel(taskID),
		"feature:   "+m.taskLabel(a.Feature),
		"pr:        "+dash(a.PR),
		"workspace: "+dash(a.Workspace),
		"container: "+m.agentContainer(a),
	)
	if a.Name == m.selID() { // dial-ins are fetched for the selected agent only
		ls = append(ls, clientLines(m.agentClients)...)
	}
	if m.tab == 1 && a.Name == m.selID() {
		ls = append(ls, "", "── activity ──")
		// Newest-first so the latest action is visible at the top.
		for i := len(m.agentLog) - 1; i >= 0; i-- {
			e := m.agentLog[i]
			ls = append(ls, fmt.Sprintf("%s  %-10s %s", dimStyle.Render(eventTime(e.TS)), e.Type, e.Payload))
		}
	}
	return ls
}

// clientLines formats dial-ins via the hub's formatter, so this matches `sindri agent info`.
func clientLines(cs []api.ClientView) []string {
	s := theme.FormatClients(cs)
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// eventTime shows a stored UTC RFC3339 stamp as local HH:MM:SS, or raw if it won't parse.
func eventTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Local().Format("15:04:05")
}
