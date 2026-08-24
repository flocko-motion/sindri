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
	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/ui/attach"
	"github.com/flo-at/sindri/internal/ui/table"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// attachCmd builds the interactive tmux attach through the container port, so any backend works.
// cname is the board's project-resolved pod (the board is multi-repo); name is the tmux session.
func attachCmd(cname, name string) *exec.Cmd {
	return container.AttachCmd(cname, append([]string{"tmux"}, tmux.Attach(name, false)...)...)
}

// attachTookHold is the shortest a real dial-in can last. Under it, the child cannot have handed the
// terminal over and back — so an error is the attach never starting, which the keypress deserves an
// answer about. Over it, a non-zero exit is the session's own business (a detach, a killed pane).
const attachTookHold = 400 * time.Millisecond

// attachAgent attaches and reports to herdr's sidebar for the duration, like every other attach
// path (no-op outside a herdr pane). Released when the child exits, before the resume repaint.
//
// It never checks liveness first. The board's status is the watchdog's last sweep, which on a loaded
// host can call a live agent down — and refusing on that costs the user the access, where attempting
// costs a moment. So it tries, and explains only if the try fails.
func attachAgent(cname, name string) tea.Cmd {
	stop := attach.ReportToHerdr(cname, name)
	began := time.Now()
	return tea.ExecProcess(attachCmd(cname, name), func(err error) tea.Msg {
		stop()
		if err != nil && time.Since(began) < attachTookHold {
			return errModalMsg{fmt.Errorf("couldn't attach to %s — it looks like its container or tmux "+
				"session isn't up. '%s' starts it; '%s' shows what the hub's probes see. (%v)",
				name, keyStartS, keyWhyNext, err)}
		}
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
	opts := []string{"worker", "reviewer", "reviewer (global)", "planner", "coauthor"}
	vals := []string{"worker", "reviewer", "global-reviewer", "planner", "coauthor"}
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
			target, role := cl, v
			if v == "global-reviewer" {
				target, role = client.Dial(api.GlobalProject), "reviewer"
			}
			// Register, then launch. The launch is a separate step so the new row appears at
			// once, but its result is collected rather than dropped: a launch can fail (no
			// image, no engine, a build that breaks) and the row would otherwise just sit at
			// "down" with nothing said.
			return func() tea.Msg {
				if target == nil {
					return nil
				}
				name, err := target.NewAgent("", role, "") // memory: hub default; editable via the detail view / CLI
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
// only account of a failure — so it is captured either way and shown when the launch fails. Sizes
// the session to the live preview pane it renders into, so it isn't cramped to tmux's 80x24
// default until someone attaches.
func (m *model) launchCmd(name string) tea.Cmd {
	cl := m.cl
	if cl == nil {
		return nil
	}
	m.flash = "launching " + name + "… (a first run builds the image, which takes a while)"
	cols, lines := m.previewSize()
	return func() tea.Msg {
		var buf bytes.Buffer
		err := cl.Launch(name, false, false, cols, lines, &buf)
		return launchedMsg{name: name, log: buf.String(), err: err}
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

// attachTo dials in without asking the board's permission first. Every tab that offers attach goes
// through it, so none of them can reintroduce a gate the others dropped.
func (m *model) attachTo(a api.AgentView) tea.Cmd {
	if m.cl == nil {
		return nil
	}
	return attachAgent(m.agentContainer(a), a.Name)
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

// previewSize is the live tmux pane's content width and height — the same numbers agentsBody
// renders the preview at, and what a freshly launched session should be created at (-> launchCmd)
// so it isn't cramped to tmux's 80x24 default until someone attaches.
func (m model) previewSize() (w, h int) {
	leftW := m.w
	if m.showDetail() { // leave room for the right detail column
		leftW = m.w - m.agentDetailWidth() - 1
	}
	return leftW, max(1, m.bodyHeight()-m.agentListHeight()-1) // minus the horizontal divider
}

// agentsBody lays out list over live pane on the left, fixed-width agent detail on the right.
func (m model) agentsBody() string {
	h := m.bodyHeight()
	rightW := m.agentDetailWidth()
	leftW, paneH := m.previewSize()

	listBox := pane(rowTexts(m.rows()), m.list, leftW, m.selRow())
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
	}
	// Unread mail, where there is any: the mailbox waits quietly by design, so a count on the agent
	// is the only thing that shows one has stopped reading.
	if a.UnreadMail > 0 {
		items = append(items, metaItem{
			text:  stWarn.Render(fmt.Sprintf("mail:      %d unread", a.UnreadMail)) + dimStyle.Render("  (⏎ read them)"),
			kind:  "mail",
			value: a.Name,
		})
	}
	// The question an escalated agent stopped on, beside the status word that says it is. Readable
	// here on purpose: several escalations can be triaged before deciding which to sit down with,
	// which attaching to each pane in turn does not allow. ⏎ clears it — the user's own release,
	// for an agent that cannot do it itself.
	if a.Escalation != "" {
		items = append(items, metaItem{
			text:  "escalated: " + a.Escalation + dimStyle.Render("  (⏎ resume)"),
			kind:  "resume",
			value: a.Name,
		})
	}
	items = append(items,
		taskIt, featIt, prIt,
		wsIt,
		metaItem{text: "memory:    " + memoryLabelTUI(a.Memory, m.state.DefaultMemory) + dimStyle.Render("  (container RAM · e to edit)")},
		metaItem{text: "context:   " + theme.ContextLine(a.ContextTokens)},
		metaItem{text: "model:     " + dash(a.Model)},
		metaItem{text: pod, kind: "view", value: "pod"},
	)
	// The armed clear says WHEN it lands, not merely that it is set: the row's marker is the count,
	// this is the sentence behind it — and "C cancels" because a toggle needs its way back shown.
	if a.ClearArmed {
		items = append(items, metaItem{text: stWarn.Render("clear:     " + clearGlyph + " armed — " +
			clearLandsWhen(a) + dimStyle.Render("  ("+keyClearCtx+" cancels)"))})
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
	case "stopped":
		return []string{dimStyle.Render("(stopped — '" + keyStartS + "' resumes the session)")}
	case "stopping":
		return []string{dimStyle.Render("stopping…")}
	case "launching":
		if hasBody { // pod is up and booting — show its startup output
			return body
		}
		return []string{dimStyle.Render("launching… (building image / starting container)")}
	case "launch-failed":
		return []string{dimStyle.Render("(launch failed — see the log; '" + keyStartS + "' tries again)")}
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

// The row markers, from the set both front-ends share, so a symbol means one thing wherever it is
// drawn (-> theme/glyph.go, which also holds why each is the width it is).
const (
	eyeGlyph       = theme.MarkDialIn
	warnGlyph      = theme.MarkWarning
	attentionGlyph = theme.MarkNeedsUser
	mailGlyph      = theme.MarkMail
	retiredGlyph   = theme.MarkRetired
	clearGlyph     = theme.MarkClearArmed
)

// agentTable is the Agents list's columns. The header and every row are laid out through it, so a
// label cannot come to sit over the wrong column.
var agentTable = table.Table{
	{Label: "repo", Width: 10, Clip: true}, // a repo name is unbounded; a long one would skew every row
	{Label: "agent", Width: 12},
	{Label: "role", Width: 8},
	{Label: "status", Width: 9},
	{Label: "ctx", Width: 4, Right: true},
	{Label: "model", Width: 14, Clip: true}, // a raw model id is unbounded and often dated
	{Label: "work"},
}

// AgentColumnLabels is agentTable's column labels, left to right — exported so a cross-front-end
// test (-> internal/ui) can pin their order against the CLI's `agent list` columns without either
// package importing the other, and without duplicating the layout each renders from.
func AgentColumnLabels() []string {
	labels := make([]string, len(agentTable))
	for i, c := range agentTable {
		labels[i] = c.Label
	}
	return labels
}

func (m model) agentRows() []row {
	var foreign, local []row
	// Ordered by repo, then role, then name — the same call `sindri agent list` makes, so the two
	// front-ends cannot drift onto different orders. The repo key gathers the stuck foreign agents;
	// the heading says they are foreign, which a skimmed repo column does not (-> sectioned).
	for _, a := range api.SortedAgents(m.state.Agents, m.state.Projects) {
		switch { // still one predicate deciding what is listed: which group is all this asks
		case m.inScope(a.Project):
			local = append(local, m.agentRow(a))
		case m.agentVisible(a): // out of scope and listed anyway = stuck on the user elsewhere
			foreign = append(foreign, m.agentRow(a))
		}
	}
	out := m.listing(agentTable, foreign, local)
	for _, o := range m.state.Orphans {
		// The id is the container name so D can remove it; agent-only actions skip
		// non-roster ids, and isOrphan gates the ones reading selID directly.
		out = append(out, row{stWarn.Render(warnGlyph + " orphan: " + o), o})
	}
	return out
}

// agentRow is one roster row: repo, name, role, lifecycle, context, work, and what is owed on it.
func (m model) agentRow(a api.AgentView) row {
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
	// The handle's marker gives a count; this is the row behind it, saying whose move it is in the
	// same words `sindri agent list` uses. Under repo scope it also answers why an agent from
	// another repo is in this list at all.
	if api.AgentNeedsUser(a) {
		task += "  " + stWarn.Render(warnGlyph+" needs you")
	}
	// Retirement rides beside the status, never in it: it is true of a busy agent too, and what
	// that agent is doing right now is the one thing the status column exists to say.
	if a.Retired {
		task += "  " + stDone.Render(retiredGlyph+" retired")
	}
	if a.UnreadMail > 0 { // it has been told things it has not read
		task += "  " + stWarn.Render(fmt.Sprintf("%s%d", mailGlyph, a.UnreadMail))
	}
	// An armed clear is a toggle, and a toggle you cannot see is worse than none: this is the
	// only thing separating an agent about to lose its session from one carrying on.
	if a.ClearArmed {
		task += "  " + stWarn.Render(clearGlyph+" clear armed")
	}
	return row{agentTable.Line(
		table.Cell{Text: a.Repo, Style: m.repoStyle(a.Project).Render},
		table.Cell{Text: a.Name, Style: ac.Render},
		table.Cell{Text: a.Role, Style: ac.Render},
		table.Cell{Text: a.Status, Style: ac.Render},
		table.Cell{Text: theme.ContextPercent(a.ContextTokens, a.ContextWindow), Style: ac.Render},
		table.Cell{Text: dash(a.Model), Style: ac.Render},
		table.Cell{Text: task, Style: ac.Render},
	), a.Name}
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
