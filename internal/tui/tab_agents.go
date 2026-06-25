// package: tui / agents
// type:    ui (Agents tab)
// job:     the Agents tab content — the agent list (status, role, task) with
//          orphan warnings, and the agent detail pane (state + the lazily-
//          fetched activity timeline). Status is one word: down|idle|working|
//          submitted (down ⇒ not running).
// limits:  renders agent state only; mutations go through the hub (-> client)
//          and assembly is the hub's (-> State).
package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/flo-at/sindri/internal/adapter/pod"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/hub"
)

// attachCmd builds the interactive `podman exec -it … tmux attach` for an agent.
// root scopes the container name to this repo.
func attachCmd(root, name string) *exec.Cmd {
	args := append([]string{"exec", "-it", hub.Container(root, name), "tmux"}, tmux.Attach(name, false)...)
	return exec.Command(pod.Binary, args...)
}

// openNewAgentChoice opens the worker|reviewer picker for a new agent. The role
// is fixed at creation — there is no way to change it later.
func (m *model) openNewAgentChoice() {
	cl := m.cl
	m.choice = choiceModalState{
		active: true, title: "new agent role",
		options: []string{"worker", "reviewer", "planner"}, values: []string{"worker", "reviewer", "planner"},
		apply: func(v string) tea.Cmd {
			// Register the identity, then auto-start its pod. Launch runs in the
			// background (it can build the image) — the hub's lifecycle + /events
			// reflect "launching" → running without blocking the new row's appearance.
			return func() tea.Msg {
				if cl == nil {
					return nil
				}
				name, err := cl.NewAgent("", v)
				if err != nil {
					return errModalMsg{err}
				}
				if name != "" {
					go func() { _ = cl.Launch(name, false) }()
				}
				st, _ := cl.State()
				return polledMsg(st)
			}
		},
	}
}

// openDeleteChoice opens the delete-agent confirm.
func (m *model) openDeleteChoice(id string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true, title: "delete agent " + id + "?",
		options: []string{"cancel", "delete"}, values: []string{"cancel", "delete"},
		apply: func(v string) tea.Cmd {
			if v != "delete" {
				return nil
			}
			return mutateThenRefresh(cl, func() { _ = cl.DeleteAgent(id) })
		},
	}
}

// agentStartStop is the Start/Stop toggle for the selected agent: start it if
// it's down, stop it if it's running, no-op while it's transitioning.
func (m *model) agentStartStop() tea.Cmd {
	a, ok := m.selAgent()
	if !ok {
		return nil
	}
	switch a.Status {
	case "down":
		m.flash = "starting " + a.Name + "…" // status (hub) drives the rest
		return m.action(func(id string) error { return m.cl.Launch(id, false) })
	case "launching", "stopping":
		m.flash = a.Name + " is " + a.Status + "…"
		return nil
	default: // running
		m.flash = "stopping " + a.Name + "…"
		return m.action(func(id string) error { return m.cl.StopAgent(id) })
	}
}

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
	leftW := m.w
	if m.showDetail() { // leave room for the right detail column
		leftW = m.w - clampInt(agentDetailW, 20, max(20, m.w-30)) - 1
	}
	listH := m.agentListHeight()
	paneH := max(1, h-listH-1) // minus the horizontal divider

	listBox := pane(rowTexts(m.rows()), m.list, leftW, m.cursor[m.tab])
	paneBox := tailPane(m.paneLines(), leftW, paneH)
	leftCol := strings.Join([]string{listBox, hdivider(leftW), paneBox}, "\n")

	if !m.showDetail() { // § hid the right column — left split takes the full width
		return leftCol
	}
	// Right column from metaItems, highlighting the focused actionable item.
	items := m.agentItems()
	lines := make([]string, len(items))
	hl, ai := -1, 0
	for i, it := range items {
		lines[i] = it.text
		if it.kind != "" {
			if m.rightFocus && ai == m.rightCursor {
				hl = i
			}
			ai++
		}
	}
	right := pane(lines, m.detail, m.w-leftW-1, hl)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, divider(h), right)
}

// agentItems is the selected agent's detail as metaItems: the agent's fields
// (task/PR are cross-references, the pod is a focusable view toggle) followed by
// the activity log. Selecting the pod field flips the main pane between the live
// tmux screen and the container's pod info.
func (m model) agentItems() []metaItem {
	a, ok := m.selAgent()
	if !ok {
		return []metaItem{{text: dimStyle.Render("(orphan — no roster entry; 'podman rm -f' it)")}}
	}
	taskIt := metaItem{text: "task:      " + dash(a.Task)}
	if a.Task != "" {
		taskIt.kind, taskIt.value = "task", a.Task
	}
	prIt := metaItem{text: "pr:        " + dash(a.PR)}
	if a.PR != "" {
		prIt.kind, prIt.value = "pr", a.PR
	}
	pod := "pod:       " + hub.Container(m.root, a.Name)
	if m.agentView == "pod" { // mark which view the main pane is showing
		pod += dimStyle.Render("  ◂ shown")
	}
	items := []metaItem{
		{text: "role:      " + a.Role},
		{text: "status:    " + a.Status},
		taskIt, prIt,
		{text: "workspace: " + dash(a.Workspace)},
		{text: pod, kind: "view", value: "pod"},
	}
	items = append(items, metaItem{text: ""}, metaItem{text: dimStyle.Render("── activity ──")})
	for i := len(m.agentLog) - 1; i >= 0; i-- { // newest-first
		e := m.agentLog[i]
		items = append(items, metaItem{text: fmt.Sprintf("%s  %-10s %s", dimStyle.Render(eventTime(e.TS)), e.Type, e.Payload)})
	}
	return items
}

// agentActionable is the focusable subset of the agent detail (view selectors +
// task/PR cross-refs).
func (m model) agentActionable() []metaItem {
	var out []metaItem
	for _, it := range m.agentItems() {
		if it.kind != "" {
			out = append(out, it)
		}
	}
	return out
}

// paneLines is the live-screen region: the captured tmux screen when running,
// otherwise a message reflecting the hub's lifecycle status.
func (m model) paneLines() []string {
	a, ok := m.selAgent()
	if !ok { // nothing selected — usually because there are no agents yet
		return []string{dimStyle.Render("(no agents)")}
	}
	if m.agentView == "pod" { // pod-info view (selected the container item)
		if strings.TrimSpace(m.agentPod) == "" {
			return []string{dimStyle.Render("(fetching pod info…)")}
		}
		return strings.Split(strings.TrimRight(m.agentPod, "\n"), "\n")
	}
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
		// Whole row coloured by lifecycle state (grey down, yellow transitioning,
		// green running); cells styled independently so resets don't bleed.
		ac := agentStatusStyle(a.Status)
		out = append(out, row{strings.Join([]string{
			ac.Render(fmt.Sprintf("%-9s", a.Status)),
			ac.Render(fmt.Sprintf("%-12s", a.Name)),
			ac.Render(fmt.Sprintf("%-8s", a.Role)),
			ac.Render(dash(a.Task)),
		}, " "), a.Name})
	}
	for _, o := range m.state.Orphans {
		out = append(out, row{stWarn.Render("⚠ orphan: " + o), ""})
	}
	return out
}

func (m model) agentDetailLines() []string {
	a, ok := m.selAgent()
	if !ok {
		return []string{dimStyle.Render("(orphan — no roster entry; 'podman rm -f' it)")}
	}
	return m.agentDetailFor(a)
}

// agentDetailFor renders an agent's detail (used for the Agents tab and, via the
// item convention, the modal-peek). The activity log is only shown for the
// currently-selected agent (it's lazily fetched for that one).
func (m model) agentDetailFor(a hub.AgentView) []string {
	ls := []string{
		"agent:     " + a.Name,
		"role:      " + a.Role,
		"status:    " + a.Status,
		"task:      " + dash(a.Task),
		"pr:        " + dash(a.PR),
		"workspace: " + dash(a.Workspace),
		"pod:       " + hub.Container(m.root, a.Name),
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

// eventTime renders an activity-log timestamp (stored UTC RFC3339) as a local
// HH:MM:SS, falling back to the raw value if it doesn't parse.
func eventTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Local().Format("15:04:05")
}
