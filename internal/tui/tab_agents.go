// package: tui / agents
// type:    ui (Agents tab)
// job:     the Agents tab content — the agent list (status, role, task) with
//          orphan warnings, and the agent detail pane (state + the lazily-
//          fetched activity timeline). Status is one word: down|idle|working|
//          submitted (down ⇒ not running).
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/flo-at/sindri/internal/hub"
)

// agentDetailW is the fixed width of the Agents tab's right detail column —
// wide enough that activity payloads (task ids + titles) aren't chopped.
const agentDetailW = 62

// agentListHeight is the height of the short agent-list region (top-left); the
// live tmux pane gets the rest of the left column.
func (m model) agentListHeight() int {
	n := len(m.rows())
	if n < 1 {
		n = 1
	}
	if cap := m.bodyHeight() * 2 / 5; n > cap { // keep it "short"
		n = max(cap, 3)
	}
	return n
}

// agentsBody renders the Agents tab: a short agent list over the live tmux pane
// on the left (main), and the fixed-width agent detail on the right.
func (m model) agentsBody() string {
	h := m.bodyHeight()
	rightW := clampInt(agentDetailW, 20, max(20, m.w-30))
	leftW := m.w - rightW - 1 // minus the vertical divider
	listH := m.agentListHeight()
	paneH := max(1, h-listH-1) // minus the horizontal divider

	listBox := pane(rowTexts(m.rows()), m.list, leftW, m.cursor[m.tab])
	paneBox := tailPane(m.paneLines(), leftW, paneH)
	leftCol := strings.Join([]string{listBox, hdivider(leftW), paneBox}, "\n")

	right := pane(m.detailLines(), m.detail, rightW, -1)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, divider(h), right)
}

// paneLines is the live-screen region: the captured tmux screen when running,
// otherwise a message reflecting the hub's lifecycle status.
func (m model) paneLines() []string {
	a, _ := m.selAgent()
	body := strings.Split(strings.TrimRight(m.agentPane, "\n"), "\n")
	hasBody := strings.TrimSpace(m.agentPane) != ""
	switch a.Status {
	case "down":
		return []string{dimStyle.Render("(not running — launch with 'L')")}
	case "stopping":
		return []string{dimStyle.Render("stopping…")}
	case "launching":
		if hasBody { // pod is up and booting — show its startup output
			return body
		}
		return []string{dimStyle.Render("launching… (building image / starting pod)")}
	default: // running
		if !hasBody {
			return []string{dimStyle.Render("(starting…)")}
		}
		return body
	}
}

// tailPane renders the last h lines of content into a width×h block.
func tailPane(lines []string, w, h int) string {
	if len(lines) > h {
		lines = lines[len(lines)-h:]
	}
	out := make([]string, 0, h)
	for _, l := range lines {
		out = append(out, padTrunc(l, w))
	}
	blank := strings.Repeat(" ", w)
	for len(out) < h {
		out = append(out, blank)
	}
	return strings.Join(out, "\n")
}

// hdivider is a horizontal rule of w columns.
func hdivider(w int) string { return divStyle.Render(strings.Repeat("─", w)) }

// selAgent returns the currently-selected agent from the board snapshot.
func (m model) selAgent() (hub.AgentView, bool) {
	id := m.selID()
	for _, a := range m.state.Agents {
		if a.Name == id {
			return a, true
		}
	}
	return hub.AgentView{}, false
}

func (m model) agentRows() []row {
	var out []row
	for _, a := range m.state.Agents {
		out = append(out, row{fmt.Sprintf("%-9s %-12s %-8s %s", a.Status, a.Name, a.Role, dash(a.Task)), a.Name})
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
		"agent:     " + a.Name,
		"role:      " + a.Role,
		"status:    " + a.Status,
		"task:      " + dash(a.Task),
		"pr:        " + dash(a.PR),
		"workspace: " + dash(a.Workspace),
		"", "── activity ──",
	}
	// Newest-first so the latest action (launch, stop, …) is visible at the top
	// of the activity section rather than scrolled off the bottom.
	for i := len(m.agentLog) - 1; i >= 0; i-- {
		e := m.agentLog[i]
		ls = append(ls, fmt.Sprintf("%s  %-10s %s", dimStyle.Render(eventTime(e.TS)), e.Type, e.Payload))
	}
	return ls
}

// eventTime renders an activity-log timestamp (stored UTC RFC3339) as a local
// HH:MM:SS, falling back to the raw value if it doesn't parse.
func eventTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Local().Format("15:04:05")
}
