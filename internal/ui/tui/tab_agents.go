// package: tui / agents
// type:    ui (Agents tab)
// job:     the Agents tab content — the agent list (status, role, task) with
// orphan warnings, and the agent detail pane (state + the lazily-
// fetched activity timeline). Status is the hub's one word, rendered as
// given ("unknown" = nothing has observed the agent yet).
// limits:  renders agent state only; mutations go through the hub (-> client)
// and assembly is the hub's (-> State).
package tui

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/ui/attach"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// attachCmd builds the interactive tmux attach through the container port, so any backend works.
// cname is the board's project-resolved pod (the board is multi-repo); name is the tmux session.
func attachCmd(cname, name string) *exec.Cmd {
	return container.AttachCmd(cname, append([]string{"tmux"}, tmux.Attach(name, false)...)...)
}

// attachAgent attaches and reports to herdr's sidebar for the duration, like every other attach
// path (no-op outside a herdr pane). Released when the child exits, before the resume repaint.
func attachAgent(cname, name string) tea.Cmd {
	stop := attach.ReportToHerdr(cname, name)
	return tea.ExecProcess(attachCmd(cname, name), func(err error) tea.Msg {
		stop()
		return resumed(err)
	})
}

// openPlanForm hands a phased brief to the planner. A textarea, not a prompt: the phrasing is all
// the planner gets. The hub refuses while that planner has a PR open, so the form needn't check.
func (m *model) openPlanForm(name string) {
	goal := newTextareaField("what to plan", "")
	cl := m.cl
	m.form.open("new plan for "+name, []field{goal}, nil, func() tea.Cmd {
		text := goal.value()
		return func() tea.Msg {
			if cl == nil || strings.TrimSpace(text) == "" {
				return nil
			}
			if err := cl.AssignPlan(name, text, ""); err != nil {
				return errModalMsg{err}
			}
			st, _ := cl.State()
			return polledMsg(st)
		}
	})
}

// openTaskPlanForm hands an existing task to a planner to work up. The task carries the brief, so
// the textarea is for whatever it does not already say, and may be left empty.
func (m *model) openTaskPlanForm(planner, taskID string) {
	extra := newTextareaField("anything to add (optional)", "")
	cl := m.cl
	m.form.open("work up "+taskID+" with "+planner, []field{extra}, nil, func() tea.Cmd {
		text := extra.value()
		return func() tea.Msg {
			if cl == nil {
				return nil
			}
			if err := cl.AssignPlan(planner, text, taskID); err != nil {
				return errModalMsg{err}
			}
			st, _ := cl.State()
			return polledMsg(st)
		}
	})
}

// agentContainer prefers the board's project-resolved name (right for any repo), falling back to
// the current repo only for an older hub that doesn't report it.
func (m model) agentContainer(a api.AgentView) string {
	if a.Container != "" {
		return a.Container
	}
	return container.AgentContainer(m.root, a.Name)
}

// memoryLabelTUI shows the RAM limit, naming the hub's own default where none is configured — the
// figure comes off the board rather than being copied here, since it is the runtime's to state.
func memoryLabelTUI(m, dflt string) string {
	if strings.TrimSpace(m) == "" {
		return theme.MemoryDefaultLabel(dflt)
	}
	return m
}

// openAgentOptionsForm edits per-agent settings — so far only the RAM limit, applied on next start.
func (m *model) openAgentOptionsForm(name, current string) {
	mem := newTextField("memory: container RAM limit (e.g. 4g, 512m; empty = default)", current)
	cl := m.cl
	m.form.open("options "+name, []field{mem}, nil, func() tea.Cmd {
		val := strings.TrimSpace(mem.value())
		return func() tea.Msg {
			if cl == nil {
				return nil
			}
			if err := cl.SetMemory(name, val); err != nil {
				return errModalMsg{err}
			}
			st, _ := cl.State()
			return polledMsg(st)
		}
	})
}

// openNewAgentChoice picks the role for a new agent; the role is fixed at creation.
func (m *model) openNewAgentChoice() {
	cl := m.cl
	opts := []string{"worker", "reviewer", "planner", "coauthor"}
	vals := []string{"worker", "reviewer", "planner", "coauthor"}
	// Plans share the "new" key rather than a second binding; only planners take one.
	planner := ""
	if a, ok := m.selAgent(); ok && a.Role == "planner" {
		planner = a.Name
		opts = append(opts, "plan for "+planner)
		vals = append(vals, "plan")
	}
	m.choice = choiceModalState{
		active: true, title: "new…",
		options: opts, values: vals,
		apply: func(v string) tea.Cmd {
			if v == "plan" {
				return func() tea.Msg { return openPlanFormMsg(planner) }
			}
			// Register, then launch. The launch is a separate step so the new row appears at
			// once, but its result is collected rather than dropped: a launch can fail (no
			// image, no engine, a build that breaks) and the row would otherwise just sit at
			// "down" with nothing said.
			return func() tea.Msg {
				if cl == nil {
					return nil
				}
				name, err := cl.NewAgent("", v, "") // memory: hub default; editable via the detail view / CLI
				if err != nil {
					return errModalMsg{err}
				}
				if name == "" {
					st, _ := cl.State()
					return polledMsg(st)
				}
				return agentCreatedMsg(name)
			}
		},
	}
}

// launchCmd starts a registered agent and keeps what the launch says. A first run builds the
// image, which is slow enough that silence reads as "nothing happened", and the build log is the
// only account of a failure — so it is captured either way and shown when the launch fails.
func (m *model) launchCmd(name string) tea.Cmd {
	cl := m.cl
	if cl == nil {
		return nil
	}
	m.flash = "launching " + name + "… (a first run builds the image, which takes a while)"
	return func() tea.Msg {
		var buf bytes.Buffer
		err := cl.Launch(name, false, false, &buf)
		return launchedMsg{name: name, log: buf.String(), err: err}
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
			return mutateThenRefresh(cl, func() error { return cl.DeleteAgent(id) })
		},
	}
}

// openClearContextChoice confirms clearing a full agent's context. Confirmed rather than done on
// the keystroke because it destroys everything the session remembers, including whatever the user
// typed into that pane — the hub refuses mid-task, so what is left to lose here is the reasoning.
func (m *model) openClearContextChoice(name string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true, title: "clear " + name + "'s context?  (its session starts empty)",
		options: []string{"cancel", "clear"}, values: []string{"cancel", "clear"},
		apply: func(v string) tea.Cmd {
			if v != "clear" {
				return nil
			}
			return mutateThenRefresh(cl, func() error { return cl.ClearContext(name) })
		},
	}
}

// rebaseAgentCmd rebases the agent's worktree onto the reference branch; git aborts on conflict.
func (m *model) rebaseAgentCmd(name string) tea.Cmd {
	cl := m.cl
	m.flash = "rebasing " + name + "…"
	return mutateThenRefresh(cl, func() error { return cl.RebaseAgent(name) })
}

// agentStartStop toggles the selected agent; a no-op while it's transitioning.
func (m *model) agentStartStop() tea.Cmd {
	a, ok := m.selAgent()
	if !ok {
		return nil
	}
	switch {
	case api.AgentNeedsLaunch(a.Status):
		// Same path as a freshly created agent: the launch keeps its output, so a failed image
		// build shows what broke rather than a bare "exit status 1". Not-yet-observed lands here
		// too — there is nothing to stop, and this is where such an agent went before the hub
		// could say so.
		return m.launchCmd(a.Name)
	case api.AgentNotUp(a.Status):
		m.flash = a.Name + " is " + a.Status + "…"
		return nil
	default: // running
		m.flash = "stopping " + a.Name + "…"
		return m.action(func(id string) error { return m.cl.StopAgent(id) })
	}
}

// attachOrArm attaches, or refuses ONCE on a status that says the agent isn't up and lets the next
// press through. Board liveness is the watchdog's last observation, minutes stale on a loaded host,
// so the user watching a pane can be right where the sweep that timed out is wrong.
func (m *model) attachOrArm(a api.AgentView, startHint string) tea.Cmd {
	if api.AgentNotUp(a.Status) && m.attachAnyway != a.Name {
		m.attachAnyway = a.Name
		m.errText = attachRefusal(a.Name, a.Status, startHint)
		return nil
	}
	m.attachAnyway = ""
	if m.cl == nil {
		return nil
	}
	return attachAgent(m.agentContainer(a), a.Name)
}

// attachRefusal says why an attach cannot happen, in one place for the three tabs that offer it.
// Not-yet-observed gets its own wording: telling the user to start an agent that may already be
// running sends them to fix the wrong thing, when the next sweep answers within seconds.
func attachRefusal(name, status, startHint string) string {
	const anyway = " Press '" + keyAttach + "' again to attach anyway — this status is the last sweep's, not a fresh look."
	switch status {
	case api.StatusUnknown:
		return "agent " + name + " hasn't been observed yet — try again in a moment." + anyway
	case "launching", "stopping":
		return "agent " + name + " is " + status + " — try again in a moment." + anyway
	}
	return "agent " + name + " is down — start it first (" + startHint + ")." + anyway
}

// agentDetailW is wide enough that activity payloads (task ids + titles) aren't chopped.
const agentDetailW = 62

// agentListHeight sizes the short list; the live tmux pane gets the rest of the left column.
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

// agentDetailWidth is the right detail column's width — the same clamp agentsBody renders at,
// shared so reclamp can size the viewport to the same wrapped line count.
func (m model) agentDetailWidth() int {
	return clampInt(agentDetailW, 20, max(20, m.w-30))
}

// agentsBody lays out list over live pane on the left, fixed-width agent detail on the right.
func (m model) agentsBody() string {
	h := m.bodyHeight()
	leftW := m.w
	rightW := m.agentDetailWidth()
	if m.showDetail() { // leave room for the right detail column
		leftW = m.w - rightW - 1
	}
	listH := m.agentListHeight()
	paneH := max(1, h-listH-1) // minus the horizontal divider

	listBox := pane(rowTexts(m.rows()), m.list, leftW, m.cursor[m.tab])
	paneBox := tailPane(m.paneLines(), leftW, paneH)
	leftCol := strings.Join([]string{listBox, hdivider(leftW), paneBox}, "\n")

	if !m.showDetail() { // § hid the right column — left split takes the full width
		return leftCol
	}
	// Right column from metaItems, word-wrapped like the PRs tab so a long task title or
	// activity payload reads in full rather than losing its tail to an ellipsis. Highlight
	// the focused actionable item.
	items := wrapMeta(m.agentItems(), rightW)
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
	right := pane(lines, m.detail, rightW, hl)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, divider(h), right)
}

// agentItems is the selected agent's fields plus activity log; task/PR are cross-references and
// the pod item is a toggle flipping the main pane between the live tmux screen and pod info.
func (m model) agentItems() []metaItem {
	a, ok := m.selAgent()
	if !ok {
		return []metaItem{{text: dimStyle.Render("(orphan — no roster entry; '" + keyDelete + "' removes it)")}}
	}
	// A reviewer's own Task is always "" — the task belongs to the agent that wrote the PR — so
	// fall back to what that PR is for, or the line reads as an agent holding nothing at all.
	taskID := a.Task
	if taskID == "" {
		taskID = m.prTask(a.PR)
	}
	taskIt := metaItem{text: "task:      " + m.taskLabel(taskID)}
	if taskID != "" {
		taskIt.kind, taskIt.value = "task", taskID
	}
	// The feature reads alongside the subtask, and carries the pane on its own between subtasks —
	// where the task line is a dash and the agent otherwise looks like it holds nothing at all.
	featIt := metaItem{text: "feature:   " + m.taskLabel(a.Feature)}
	if a.Feature != "" {
		featIt.kind, featIt.value = "task", a.Feature
	}
	prIt := metaItem{text: "pr:        " + dash(a.PR)}
	if a.PR != "" {
		prIt.kind, prIt.value = "pr", a.PR
	}
	pod := "container: " + m.agentContainer(a)
	if m.agentView == "pod" { // mark which view the main pane is showing
		pod += dimStyle.Render("  ◂ shown")
	}
	status := "status:    " + a.Status
	if m.agentView == "diag" {
		status += dimStyle.Render("  ◂ shown")
	} else {
		status += dimStyle.Render("  (⏎ why)")
	}
	// Absolute: `value` is a child process's working directory and what `y` copies, so the text
	// shows that same string. With no project root to join to, the relative form still shows — the
	// field holds its place — but plain, since a shell opened at a relative path lands anywhere.
	wsIt := metaItem{text: "workspace: " + dash(a.Workspace)}
	if ws := m.agentWorkspacePath(a.Name); ws != "" {
		wsIt = metaItem{text: "workspace: " + ws, kind: "path", value: ws}
	}
	items := []metaItem{
		{text: "role:      " + a.Role},
		{text: status, kind: "view", value: "diag"},
		taskIt, featIt, prIt,
		wsIt,
		{text: "memory:    " + memoryLabelTUI(a.Memory, m.state.DefaultMemory) + dimStyle.Render("  (container RAM · e to edit)")},
		{text: "context:   " + theme.ContextLine(a.ContextTokens)},
		{text: pod, kind: "view", value: "pod"},
	}
	for _, line := range clientLines(m.agentClients) { // same dial-in detail as `agent info`
		items = append(items, metaItem{text: line})
	}
	items = append(items, metaItem{text: ""}, metaItem{text: dimStyle.Render("── activity ──")})
	for i := len(m.agentLog) - 1; i >= 0; i-- { // newest-first
		e := m.agentLog[i]
		items = append(items, metaItem{text: fmt.Sprintf("%s  %-10s %s", dimStyle.Render(eventTime(e.TS)), e.Type, e.Payload)})
	}
	return items
}

// agentActionable is the focusable subset of the agent detail.
func (m model) agentActionable() []metaItem {
	var out []metaItem
	for _, it := range m.agentItems() {
		if it.kind != "" {
			out = append(out, it)
		}
	}
	return out
}

// paneLines is the captured tmux screen when running, else the hub's lifecycle status.
func (m model) paneLines() []string {
	a, ok := m.selAgent()
	if !ok { // nothing selected — usually because there are no agents yet
		return []string{dimStyle.Render("(no agents)")}
	}
	if m.agentView == "pod" { // pod-info view (selected the container item)
		if strings.TrimSpace(m.agentPod) == "" {
			return []string{dimStyle.Render("(fetching container info…)")}
		}
		return strings.Split(strings.TrimRight(m.agentPod, "\n"), "\n")
	}
	if m.agentView == "diag" { // what the hub's liveness probes actually observe
		if strings.TrimSpace(m.agentDiag) == "" {
			return []string{dimStyle.Render("(asking the hub why…)")}
		}
		head := dimStyle.Render("liveness probe — why status is " + strconv.Quote(a.Status) + ":")
		return append([]string{head}, strings.Split(strings.TrimRight(m.agentDiag, "\n"), "\n")...)
	}
	body := strings.Split(strings.TrimRight(m.agentPane, "\n"), "\n")
	hasBody := strings.TrimSpace(m.agentPane) != ""
	switch a.Status {
	case "down":
		// Built from the key constant, not spelled out: this said 'L', which is lint on the PRs
		// tab and bound to nothing here, so the one hint a stopped agent shows led nowhere.
		return []string{dimStyle.Render("(not running — start with '" + keyStartS + "')")}
	case "stopping":
		return []string{dimStyle.Render("stopping…")}
	case "launching":
		if hasBody { // pod is up and booting — show its startup output
			return body
		}
		return []string{dimStyle.Render("launching… (building image / starting container)")}
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
func (m model) selAgent() (api.AgentView, bool) {
	id := m.selID()
	for _, a := range m.state.Agents {
		if a.Name == id {
			return a, true
		}
	}
	return api.AgentView{}, false
}

// eyeGlyph marks attached humans. The U+FE0F is load-bearing: bare U+1F441 measures one cell but
// draws two, and one cell of overflow makes JoinHorizontal push the whole frame off-screen.
const eyeGlyph = "👁️"

// warnGlyph is the warning mark, likewise width-pinned.
const warnGlyph = "⚠️"

// gateGlyph marks work held back by the approval gate. Plain ASCII: it sits inside the header bar,
// where an emoji's two drawn cells against one measured would shear the whole strip.
const gateGlyph = "!"

func (m model) agentRows() []row {
	var visible []api.AgentView
	for _, a := range m.state.Agents {
		if m.inScope(a.Project) { // repo-scoped: only the active repo's agents
			visible = append(visible, a)
		}
	}
	var out []row
	// Ordered by repo, then role, then name — the same call `sindri agent list` makes, so the two
	// front-ends cannot drift onto different orders. In repo scope every row shares one repo, so
	// this reduces to role-then-name without a redundant, single-value grouping level.
	for _, a := range api.SortedAgents(visible, m.state.Projects) {
		// Row coloured by lifecycle; cells styled independently so resets don't bleed.
		ac := agentStatusStyle(a.Status)
		// Work cell: the task, or the reviewed PR since a reviewer holds no task — named alongside
		// the task that PR is FOR, since the PR id alone says nothing a human recognizes. A held
		// feature is named either way — as the subtask's parent, or alone between subtasks, where
		// showing nothing made an agent that refused every verb look plainly idle.
		work := a.Task
		if work == "" {
			work = a.PR
			if t := m.prTask(a.PR); t != "" {
				work += " › " + m.taskLabel(t)
			}
		}
		switch {
		case a.Feature != "" && work != "":
			work = a.Feature + " › " + work
		case a.Feature != "":
			work = a.Feature
		}
		task := dash(work)
		if a.Clients > 0 { // dial-ins attached — show the eye like the CLI list
			task += fmt.Sprintf("  %s%d", eyeGlyph, a.Clients)
		}
		out = append(out, row{strings.Join([]string{
			m.repoStyle(a.Project).Render(fmt.Sprintf("%-10.10s", a.Repo)),
			ac.Render(fmt.Sprintf("%-9s", a.Status)),
			ac.Render(fmt.Sprintf("%-12s", a.Name)),
			ac.Render(fmt.Sprintf("%-8s", a.Role)),
			ac.Render(fmt.Sprintf("%4s", theme.ContextPercent(a.ContextTokens, a.ContextWindow))),
			ac.Render(task),
		}, " "), a.Name})
	}
	for _, o := range m.state.Orphans {
		// The id is the container name so D can remove it; agent-only actions skip
		// non-roster ids, and isOrphan gates the ones reading selID directly.
		out = append(out, row{stWarn.Render(warnGlyph + " orphan: " + o), o})
	}
	return out
}

// isOrphan reports a stray container rather than a roster agent, routing D to orphan removal.
func (m model) isOrphan(id string) bool {
	for _, o := range m.state.Orphans {
		if o == id {
			return true
		}
	}
	return false
}

// openRemoveOrphanChoice confirms a direct container rm; there's no agent identity to delete.
func (m *model) openRemoveOrphanChoice(name string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true, title: "remove orphan container " + name + "?",
		options: []string{"cancel", "remove"}, values: []string{"cancel", "remove"},
		apply: func(v string) tea.Cmd {
			if v != "remove" {
				return nil
			}
			return mutateThenRefresh(cl, func() error { return cl.RemoveOrphan(name) })
		},
	}
}

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
		"task:      " + m.taskLabel(taskID),
		"feature:   " + m.taskLabel(a.Feature),
		"pr:        " + dash(a.PR),
		"workspace: " + dash(a.Workspace),
		"container: " + m.agentContainer(a),
	}
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
