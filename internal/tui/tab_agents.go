// package: tui / agents
// type:    ui (Agents tab)
// job:     the Agents tab content — the agent list (running marker, role, phase,
//          task) with orphan warnings, and the agent detail pane (state + the
//          lazily-fetched activity timeline).
package tui

import (
	"fmt"

	"github.com/flo-at/sindri/internal/hub"
)

func (m model) agentRows() []row {
	var out []row
	for _, a := range m.state.Agents {
		run := "·"
		if a.Running {
			run = "●"
		}
		out = append(out, row{fmt.Sprintf("%s %-12s %-8s %-10s %s", run, a.Name, a.Role, a.Phase, dash(a.Task)), a.Name})
	}
	for _, o := range m.state.Orphans {
		out = append(out, row{"⚠ orphan: " + o, ""})
	}
	return out
}

func (m model) agentDetailLines() []string {
	id := m.selID()
	if id == "" {
		return []string{dimStyle.Render("(orphan — no roster entry; 'podman rm -f' it)")}
	}
	var a hub.AgentView
	for _, x := range m.state.Agents {
		if x.Name == id {
			a = x
		}
	}
	ls := []string{
		"agent:   " + a.Name,
		"role:    " + a.Role,
		fmt.Sprintf("running: %v", a.Running),
		"phase:   " + a.Phase,
		"task:    " + dash(a.Task),
		"pr:      " + dash(a.PR),
		"", "── activity ──",
	}
	for _, e := range m.agentLog {
		ls = append(ls, fmt.Sprintf("%-10s %s", e.Type, e.Payload))
	}
	return ls
}
