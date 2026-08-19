// package: tui / tasks
// type:    ui (Tasks tab)
// job:     the Tasks tab content — the hierarchical tree selector (collapsible,
// PR-marked) and its actions (new/edit/close/approve/reject/…). Which tasks a
// filter admits and how they arrange come from the exchange package
// (FilterTasks, ArrangeTasks); this renders rows and folds.
// limits:  renders rows and folds, and wires actions; the detail pane's items are
// tab_tasks_detail.go's, and the filter rule and tree arrangement are shared
// with the CLI (-> api.MatchesFilter, api.ArrangeTasks).
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/ui/table"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// taskTable is the Tasks list's columns. The tree gutter and the marker column go unlabelled: both
// are a couple of cells wide, and two characters cannot name "an agent is on this, and it has a PR"
// — a cryptic label would be worse than the glyphs it sat over, which the detail pane explains.
var taskTable = table.Table{
	{Width: treeGutterW},
	{Label: "id", Width: 9},
	{Label: "type", Width: 5},
	{Label: "prio", Width: 8},
	{Label: "tier", Width: 6},
	{Label: "state", Width: 8},
	{Label: "age", Width: 4, Right: true},
	{Width: marksW},
	{Label: "title"},
}

// taskRows builds the filtered, folded, depth-indented task tree. Which tasks the filter admits is
// the exchange package's answer (-> api.MatchesFilter), the same one `sindri task list --filter`
// gets, so the two front-ends cannot come to mean different things by the same word.
func (m model) taskRows() []row {
	tasks := api.FilterTasks(m.filter, m.state.Tasks)
	// Search narrows WITHIN the filter (a match the filter already excluded stays excluded), but a
	// match's ancestors come back from the whole board regardless of the filter — the same
	// exemption sd-c0a7a0 gives the status filters' own ancestors, so a match never sits at an
	// indentation lying about where it hangs.
	term := searchTerm(m.taskSearch)
	searching := term != ""
	var context map[string]bool
	if searching {
		tasks, context = api.WithAncestors(matchTasks(tasks, term), m.state.Tasks)
	}
	arranged := api.ArrangeTasks(tasks, m.state.PRs)

	// Who is behind each task (drives the worked-on marker). The same rule the detail pane names
	// the agent by, so the mark and the name cannot contradict each other.
	assigned := api.AgentsByTask(m.state.Agents, m.state.PRs)
	// Hub-side approval per task (drives the row colour for planner proposals). A gate on a task
	// that has ended is spent, and the state word below is the status's to give.
	approval := map[string]string{}
	for _, t := range m.state.Tasks {
		if t.Approval != "" && !api.DoneStatus(t.Status) {
			approval[t.ID] = t.Approval
		}
	}
	// Read from the WHOLE board, never the filtered set: an ancestor that releases the tree may be
	// closed and out of view, and its children would otherwise read as held back.
	released := api.ReleasedByPriority(m.state.Tasks)

	// Children: a later row one level deeper, before the depth returns to this level.
	hasKids := map[string]bool{}
	for i, tr := range arranged {
		for _, c := range arranged[i+1:] {
			if c.Depth <= tr.Depth {
				break
			}
			if c.Depth == tr.Depth+1 {
				hasKids[tr.ID] = true
				break
			}
		}
	}

	// Visible set after applying folds.
	var visible []api.TaskRow
	hideAbove := -1 // depth of a collapsed ancestor; rows deeper than this are hidden
	for _, tr := range arranged {
		if hideAbove >= 0 && tr.Depth > hideAbove {
			continue
		}
		hideAbove = -1
		visible = append(visible, tr)
		// A fold set before the search must not hide a match found after it.
		if hasKids[tr.ID] && m.collapsed[tr.ID] && !searching {
			hideAbove = tr.Depth
		}
	}

	// The tree lives entirely in a fixed-width gutter, so the id and later columns stay aligned.
	out := make([]row, len(visible))
	last := lastSiblings(visible)
	cont := []bool{} // cont[i]: ancestor at depth i has a later sibling (draw │)
	for i, tr := range visible {
		if len(cont) > tr.Depth {
			cont = cont[:tr.Depth]
		}
		gutter := treeGutter(cont, tr.Depth, last[i], hasKids[tr.ID], m.collapsed[tr.ID])
		cont = append(cont, !last[i])

		// Cells styled independently (never nested) so a colour reset can't bleed across the row.
		sc, state := taskRowStyle(tr.Task, approval[tr.ID], released[tr.ID])
		if context[tr.ID] { // present for the path to a match, not a match itself
			sc = dimStyle
		}
		if v := m.busy[tr.ID]; v != "" { // transient: the user triggered a close/scrap, awaiting the hub
			sc, state = stWarn, v
		}
		prio := sc // critical priority is pink, in its own column, whatever the row's state
		if isCriticalPriority(tr.Priority) {
			prio = stPrio
		}
		out[i] = row{
			taskTable.Line(
				table.Cell{Text: gutter},
				table.Cell{Text: tr.ID, Style: sc.Render},
				table.Cell{Text: typeAbbr(tr.Type), Style: sc.Render},
				table.Cell{Text: theme.PriorityLabel(tr.Priority), Style: prio.Render},
				table.Cell{Text: api.TierOrDefault(tr.Tier), Style: sc.Render},
				table.Cell{Text: state, Style: sc.Render},
				// Age, right-aligned so the units line up under each other; the exact moment is in
				// the detail pane, which is where a question about one task gets asked.
				table.Cell{Text: theme.Age(tr.CreatedAt), Style: sc.Render},
				table.Cell{Text: taskMarks(assigned[tr.ID] != "", prMarkKind(tr)), Style: sc.Render},
				table.Cell{Text: tr.Title, Style: sc.Render},
			),
			tr.ID,
		}
	}
	return m.listing(taskTable, nil, out)
}

// searchTerm normalizes free-typed search text for matching: trimmed and lowered, "" meaning no
// search is active. Not Tasks-specific, so another list tab's own field can share it.
func searchTerm(raw string) string { return strings.ToLower(strings.TrimSpace(raw)) }

// matchesSearch reports whether a normalized term is a substring of id or title, case-insensitive
// — the one matching rule every searchable list tab shares, so wiring a second tab to it later
// needs no rule of its own.
func matchesSearch(term, id, title string) bool {
	return strings.Contains(strings.ToLower(id), term) || strings.Contains(strings.ToLower(title), term)
}

// matchTasks keeps the tasks whose id or title contains term.
func matchTasks(tasks []api.Task, term string) []api.Task {
	var out []api.Task
	for _, t := range tasks {
		if matchesSearch(term, t.ID, t.Title) {
			out = append(out, t)
		}
	}
	return out
}

// taskRowStyle is a row's colour and its state word. The WORD comes from the gate holding it where
// one is, else its status; RED is decided separately and last, by asking api.TaskNeedsUser — the
// very predicate the Tasks badge counts. Deciding it in the switch instead let the two part company
// on case order alone: a rejected task with no rating matched "rejected" and rendered grey while
// the badge, reading the rating, counted it. gated is the task's approval state, "" where no gate
// applies (a finished task's spent one included); released says whether a priority lets a worker
// take it.
func taskRowStyle(t api.Task, gated string, released bool) (lipgloss.Style, string) {
	style, word := taskStatusStyle(t.Status), theme.StateLabel(t.Status)
	switch {
	case gated == "pending":
		word = theme.ApprovalLabel("pending")
	case gated == "rejected":
		// The user has ruled; it is the author's move, so this asks nothing of anybody here.
		style, word = stDone, theme.ApprovalLabel("rejected")
	case api.Open(t) && !released:
		// Unrated reads like ungated: both mean no worker can be given this, and a row saying plain
		// "open" claimed otherwise. A rated ancestor releases the whole tree, so only a task with
		// none anywhere above it is really held back.
		word = "unrated"
	}
	if api.TaskNeedsUser(t, released) {
		style = stCrit
	}
	return style, word
}

const treeGutterW = 6 // fits ~3 levels of "│ "/"├─" connectors

// lastSiblings marks each row that has no later sibling — what the tree connectors are drawn from.
//
// Derived here rather than carried on the wire: it is a fact about the rows as ARRANGED, and the
// arrangement the TUI draws is the visible one, with collapsed subtrees removed. Hiding a subtree
// never removes a sibling, so the answer is the same either way — and only the drawing needs it.
func lastSiblings(rows []api.TaskRow) []bool {
	out := make([]bool, len(rows))
	var later []bool // later[d]: a row at depth d follows, with no shallower row between
	for i := len(rows) - 1; i >= 0; i-- {
		d := rows[i].Depth
		for len(later) <= d {
			later = append(later, false)
		}
		out[i] = !later[d]
		later[d] = true
		later = later[:d+1] // rows below this one at greater depth are its own subtree
	}
	return out
}

// treeGutter draws ancestor pipes, the branch into this node, and any fold marker.
func treeGutter(cont []bool, depth int, last, kids, collapsed bool) string {
	var b strings.Builder
	for i := 0; i < depth; i++ {
		switch {
		case i < depth-1:
			// pipe while the path node at depth i+1 has siblings below this row
			if i+1 < len(cont) && cont[i+1] {
				b.WriteString("│ ")
			} else {
				b.WriteString("  ")
			}
		case last:
			b.WriteString("└─")
		default:
			b.WriteString("├─")
		}
	}
	s := b.String()
	if kids { // fold indicator replaces the trailing dash
		if collapsed {
			s += "▸"
		} else {
			s += "▾"
		}
	}
	return padTrunc(s, treeGutterW)
}

// marksW pads the marker column so titles line up whatever a row carries. Measured from the marks
// themselves rather than written down: a glyph swap that changed the count silently would knock
// every title out of line, and this column has now been through one.
var marksW = lipgloss.Width(theme.MarkAssigned) + lipgloss.Width(theme.MarkPRFinal)

// prMarkKind picks which PR marker a row carries: final, interim, or "" for none. A kindless PR
// defaults to final, the historical default, so older PRs still read as one.
func prMarkKind(tr api.TaskRow) string {
	if tr.PR == "" {
		return ""
	}
	if tr.PRKind == "interim" {
		return "interim"
	}
	return "final"
}

// taskMarks is the status-marker column: the worked-on mark when an agent is on the task, then the
// final or interim PR mark, padded to marksW so rows line up whatever they carry.
func taskMarks(assigned bool, prKind string) string {
	s := ""
	if assigned {
		s += theme.MarkAssigned
	}
	switch prKind {
	case "final":
		s += theme.MarkPRFinal
	case "interim":
		s += theme.MarkPRInterim
	}
	return padTrunc(s, marksW)
}

// typeAbbr shortens a td type to fit the column.
func typeAbbr(t string) string {
	if t == "feature" {
		return "feat"
	}
	if len(t) > 5 {
		return t[:5]
	}
	return t
}

// taskTypes is the full set of td issue types (display == td value).
var taskTypes = []string{"task", "feature", "bug", "epic", "chore"}

// selTask returns the currently-selected task from the board snapshot.
func (m model) selTask() (api.Task, bool) {
	id := m.selID()
	for _, t := range m.state.Tasks {
		if t.ID == id {
			return t, true
		}
	}
	return api.Task{}, false
}

// openTaskForm opens the new/edit task form. t must be freshly fetched, not a board row, or a
// save blanks the fields the board doesn't carry. Openspec items honour priority only (hub-side).
func (m *model) openTaskForm(edit bool, t api.Task) {
	prioCodes := make([]string, len(theme.PriorityWords))
	for i, w := range theme.PriorityWords {
		prioCodes[i] = theme.PriorityCode(w)
	}
	title, typ, prio, tier, parent, labels, desc, id := "", "task", "P2", "mid", "", "", "", ""
	if edit {
		id, title, prio, parent, labels, desc = t.ID, t.Title, t.Priority, t.ParentID, t.Labels, t.Description
		tier = api.TierOrDefault(t.Tier)
		if t.Type != "" {
			typ = t.Type
		}
		if prio == "" {
			prio = "P2"
		}
	}
	titleF := newTextField("title", title)
	typeF := newChoiceField("type", taskTypes, taskTypes, typ)
	prioF := newChoiceField("priority", theme.PriorityWords, prioCodes, prio)
	tierF := newChoiceField("tier", api.TierWords, api.TierWords, tier)
	parentF := newTextField("parent", parent)
	labelsF := newTextField("labels", labels)
	descF := newTextareaField("description", desc)
	heading := "new task"
	if edit {
		heading = "edit " + id
	}
	cl, known := m.cl, m.taskIDs()
	validate := func() string {
		p := strings.TrimSpace(parentF.value())
		switch {
		case p == "":
			return "" // no parent ⇒ a root task
		case p == id:
			return "a task can't be its own parent"
		case !known[p]:
			return "unknown parent: " + p
		}
		return ""
	}
	m.form.open(heading, []field{titleF, typeF, prioF, tierF, parentF, labelsF, descF}, validate, func() tea.Cmd {
		spec := api.TaskSpec{
			Title: titleF.value(), Type: typeF.value(), Priority: prioF.value(), Tier: tierF.value(),
			Parent: strings.TrimSpace(parentF.value()), Description: descF.value(), Labels: csv(labelsF.value()),
		}
		return func() tea.Msg {
			if cl == nil {
				return nil
			}
			var err error
			if edit {
				err = cl.EditTask(id, spec)
			} else {
				_, err = cl.CreateTask(spec)
			}
			if err != nil {
				return errModalMsg{err} // surface it — never swallow (e.g. td's title rules)
			}
			st, _ := cl.State()
			return polledMsg(st)
		}
	})
}

// taskGated reports a proposal still under the approval gate — the only state A/R act on. "Still"
// includes being live: a verdict on a task that has already ended decides nothing.
func (m model) taskGated() bool {
	t, ok := m.selTask()
	return ok && !api.DoneStatus(t.Status) && (t.Approval == "pending" || t.Approval == "rejected")
}

// unassignTaskCmd returns the task to the backlog; the hub refuses if a live agent holds it.
func (m *model) unassignTaskCmd(id string) tea.Cmd {
	cl := m.cl
	m.flash = "unassigning " + id + "…"
	return func() tea.Msg {
		if cl == nil {
			return nil
		}
		if err := cl.UnassignTask(id); err != nil {
			return errModalMsg{err}
		}
		st, _ := cl.State()
		return polledMsg(st)
	}
}

// openBriefChoice picks which planner works up the selected task. Named for what it hands over: the
// task carries the brief, so this chooses the reader rather than what to read.
func (m *model) openBriefChoice(taskID string) {
	var names []string
	for _, a := range m.state.Agents {
		if a.Role == "planner" && m.inScope(a.Project) {
			names = append(names, a.Name)
		}
	}
	if len(names) == 0 {
		m.errText = "no planner in this repo — `sindri agent new --role planner` first"
		return
	}
	m.choice = choiceModalState{
		active: true, title: "work up " + taskID + " with…",
		options: names, values: names,
		apply: func(v string) tea.Cmd {
			return func() tea.Msg { return openTaskPlanFormMsg{planner: v, task: taskID} }
		},
	}
}

// whyNextCmd asks the hub what it would hand out next to an agent of role, and why nothing else,
// shown as a notice — the same account the CLI prints, since the reasoning is the hub's and neither
// front-end gets to have its own version of it. The role is the tab's: the Tasks tab asks the
// backlog question, the PRs tab the reviewer's, each about the pool it is showing.
func (m *model) whyNextCmd(role string) tea.Cmd {
	cl := m.cl
	if cl == nil {
		return nil
	}
	return func() tea.Msg {
		x, err := cl.NextTask("", role)
		if err != nil {
			return taskOpDoneMsg{err: err}
		}
		return noticeMsg(theme.FormatNext(x))
	}
}

// closeTaskCmd marks the task done, showing a transient "closing" until the hub confirms.
func (m *model) closeTaskCmd(id string) tea.Cmd {
	m.markBusy(id, "closing")
	return finishTaskCmd(m.cl, m.cl.CloseTask, id, "", false)
}

// markBusy sets a transient verb so the row reflects the op before the hub confirms.
func (m *model) markBusy(id, verb string) {
	if m.busy == nil {
		m.busy = map[string]string{}
	}
	m.busy[id] = verb
}

// taskOpDone drops the transient verb, then applies the fresh board or surfaces the error. A chained
// follow-up runs last, so whatever it opens reads the board this op produced.
func (m model) taskOpDone(msg taskOpDoneMsg) (tea.Model, tea.Cmd) {
	delete(m.busy, msg.id)
	if msg.err != nil {
		m.errText = msg.err.Error()
		return m, nil
	}
	m.state = msg.state
	m.reclamp()
	return m, tea.Batch(m.syncDetail(), m.agentLiveCmds(), msg.then)
}

// reconcileBusy clears verbs a fresh board already confirms, so a marker can't linger when the
// change arrives by SSE push instead of this client's own taskOpDoneMsg.
func (m *model) reconcileBusy() {
	if len(m.busy) == 0 {
		return
	}
	present := map[string]string{}
	for _, t := range m.state.Tasks {
		present[t.ID] = t.Status
	}
	for id := range m.busy {
		if status, ok := present[id]; !ok || api.DoneStatus(status) {
			delete(m.busy, id)
		}
	}
}

// openCloseChoice offers to discard an open PR alongside; with no PR, keyClose closes directly.
func (m *model) openCloseChoice(id, pr string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true, title: "close " + id + "?  (has open PR " + pr + ")",
		options: []string{"cancel", "close task only", "close task + scrap PR " + pr},
		values:  []string{"cancel", "task", "taskpr"},
		apply: func(v string) tea.Cmd {
			switch v {
			case "task":
				return taskOpTrigger(id, "closing", finishTaskCmd(cl, cl.CloseTask, id, pr, false))
			case "taskpr":
				return taskOpTrigger(id, "closing", finishTaskCmd(cl, cl.CloseTask, id, pr, true))
			default:
				return nil
			}
		},
	}
}

// taskOpTrigger routes an op through Update so the verb gets marked first — a choice's apply
// can't mutate the model itself.
func taskOpTrigger(id, verb string, run tea.Cmd) tea.Cmd {
	return func() tea.Msg { return taskOpMsg{id: id, verb: verb, run: run} }
}

// finishTaskCmd runs one task op, optionally scraps the PR alongside, then refreshes once so both
// changes land in one snapshot. A failed task op skips the PR scrap.
func finishTaskCmd(cl *client.HTTP, taskOp func(string) error, id, prID string, alsoPR bool) tea.Cmd {
	return func() tea.Msg {
		if cl == nil {
			return nil
		}
		if err := taskOp(id); err != nil {
			return taskOpDoneMsg{id: id, err: err}
		}
		if alsoPR {
			if err := cl.ScrapPR(prID); err != nil {
				return taskOpDoneMsg{id: id, err: err}
			}
		}
		st, _ := cl.State()
		return taskOpDoneMsg{id: id, state: st}
	}
}

// approveTaskCmd clears the approval gate, making the task claimable; subtree carries the verdict
// to the proposals under it (-> openApproveChoice), and then runs next (-> priorityAfterApprove).
func approveTaskCmd(cl *client.HTTP, id string, subtree bool, then tea.Cmd) tea.Cmd {
	approve := func(string) error { return cl.ApproveTask(id, subtree) }
	return afterTaskOp(finishTaskCmd(cl, approve, id, "", false), then)
}

// afterTaskOp chains a follow-up onto a task op: the op's own result still travels, so the board
// refreshes and a failure still surfaces, and the follow-up rides along to be run once it has landed.
// Chaining the two actions rather than combining them keeps one approve path and one priority path.
func afterTaskOp(op tea.Cmd, then tea.Cmd) tea.Cmd {
	if then == nil {
		return op
	}
	return func() tea.Msg {
		msg := op()
		done, ok := msg.(taskOpDoneMsg)
		if !ok || done.err != nil {
			return msg // a failed approve releases nothing, so there is nothing to rate
		}
		done.then = then
		return done
	}
}

// openTaskRejectForm rejects a proposal with a comment, delivered to the planner.
func (m *model) openTaskRejectForm(id string) {
	reason := newTextareaField("reason", "")
	cl := m.cl
	m.form.open("reject task "+id, []field{reason}, nil, func() tea.Cmd {
		text := reason.value()
		return func() tea.Msg {
			if cl == nil || strings.TrimSpace(text) == "" {
				return nil
			}
			if err := cl.RejectTask(id, text); err != nil {
				return errModalMsg{err}
			}
			st, _ := cl.State()
			return polledMsg(st)
		}
	})
}

// taskIDs is the set of known task ids (for parent validation).
func (m model) taskIDs() map[string]bool {
	ids := make(map[string]bool, len(m.state.Tasks))
	for _, t := range m.state.Tasks {
		ids[t.ID] = true
	}
	return ids
}

// csv splits a comma-separated field value into trimmed labels.
func csv(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
