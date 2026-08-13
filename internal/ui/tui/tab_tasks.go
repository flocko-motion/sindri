// package: tui / tasks
// type:    ui (Tasks tab)
// job:     the Tasks tab content — the hierarchical tree selector (collapsible,
// PR-marked) and the task detail pane. Which tasks a filter admits and how
// they arrange come from the exchange package (FilterTasks, ArrangeTasks);
// this renders rows and folds.
// limits:  renders rows and folds only; the filter rule and the tree arrangement are
// shared with the CLI (-> api.MatchesFilter, api.ArrangeTasks).
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// taskRows builds the filtered, folded, depth-indented task tree. Which tasks the filter admits is
// the exchange package's answer (-> api.MatchesFilter), the same one `sindri task list --filter`
// gets, so the two front-ends cannot come to mean different things by the same word.
func (m model) taskRows() []row {
	arranged := api.ArrangeTasks(api.FilterTasks(m.filter, m.state.Tasks), m.state.PRs)

	// Which tasks have a worker on them right now (drives the 🔨 marker).
	assigned := map[string]bool{}
	for _, a := range m.state.Agents {
		if a.Task != "" {
			assigned[a.Task] = true
		}
	}
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
		if hasKids[tr.ID] && m.collapsed[tr.ID] {
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
		if v := m.busy[tr.ID]; v != "" { // transient: the user triggered a close/scrap, awaiting the hub
			sc, state = stWarn, v
		}
		prio := sc.Render(fmt.Sprintf("%-8s", theme.PriorityLabel(tr.Priority)))
		if isCriticalPriority(tr.Priority) {
			prio = stPrio.Render(fmt.Sprintf("%-8s", theme.PriorityLabel(tr.Priority)))
		}
		out[i] = row{
			strings.Join([]string{
				gutter,
				sc.Render(fmt.Sprintf("%-9s", tr.ID)),
				sc.Render(fmt.Sprintf("%-5s", typeAbbr(tr.Type))),
				prio,
				sc.Render(fmt.Sprintf("%-8s", state)),
				// Age, right-aligned so the units line up under each other; the exact moment is in
				// the detail pane, which is where a question about one task gets asked.
				sc.Render(fmt.Sprintf("%4s", theme.Age(tr.CreatedAt))),
				sc.Render(taskMarks(assigned[tr.ID], prMarkKind(tr))),
				sc.Render(tr.Title),
			}, " "),
			tr.ID,
		}
	}
	return out
}

// taskRowStyle is a row's colour and its state word: the gate holding it where one is, else its
// status. RED is api.TaskNeedsUser — the same predicate the Tasks badge counts — so a red row is
// always counted and a counted row always red. A rejected task is grey: the user has ruled and it
// is the author's move. gated is the task's approval state, "" when no gate applies (a finished
// task's spent one included); released says whether any priority above it lets a worker take it.
func taskRowStyle(t api.Task, gated string, released bool) (lipgloss.Style, string) {
	switch {
	case gated == "pending":
		return stCrit, theme.ApprovalLabel("pending")
	case gated == "rejected":
		return stDone, theme.ApprovalLabel("rejected")
	case api.Open(t) && !released:
		// Unrated reads like ungated: both mean no worker can be given this, and a row saying plain
		// "open" claimed otherwise. A rated ancestor releases the whole tree, so only a task with
		// none anywhere above it is really held back.
		return stCrit, "unrated"
	}
	return taskStatusStyle(t.Status), theme.StateLabel(t.Status)
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

// marksW pads the marker column: 🔨 is two cells, so a fixed width keeps titles aligned.
const marksW = 3

// prMarkKind picks the PR marker: ◆ final, ◇ interim, "" none. A kindless PR defaults to
// final, the historical default, so older PRs still show ◆.
func prMarkKind(tr api.TaskRow) string {
	if tr.PR == "" {
		return ""
	}
	if tr.PRKind == "interim" {
		return "interim"
	}
	return "final"
}

// taskMarks is the status-marker column: 🔨 when a worker is on the task, then ◆ for a final PR
// or ◇ for an interim one, padded to a fixed width so rows line up whatever they carry.
func taskMarks(assigned bool, prKind string) string {
	s := ""
	if assigned {
		s += "🔨"
	}
	switch prKind {
	case "final":
		s += "◆"
	case "interim":
		s += "◇"
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

// taskDetailLines renders the selected task, description included once the lazy fetch lands.
func (m model) taskDetailLines() []string {
	if m.selID() == "" {
		return []string{dimStyle.Render("(no task)")}
	}
	return itemTexts(m.taskItems())
}

// taskItems is the selected task's detail; parent/agent/pr are focusable cross-references.
func (m model) taskItems() []metaItem {
	id := m.selID()
	var t api.Task
	for _, x := range m.state.Tasks {
		if x.ID == id {
			t = x
		}
	}
	// The board row's description shows at once; the lazy read then refines it.
	desc := t.Description
	var comments []api.Comment
	if m.taskDetail.ID == id {
		if m.taskDetail.Description != "" {
			desc = m.taskDetail.Description
		}
		comments = m.taskDetail.Comments
	}
	return m.taskItemsFor(t, desc, comments)
}

func (m model) taskActionable() []metaItem {
	var out []metaItem
	for _, it := range m.taskItems() {
		if it.kind != "" {
			out = append(out, it)
		}
	}
	return out
}

// taskDetailFor renders any task's detail block, for the modal-peek and PRs' linked-task modal.
func (m model) taskDetailFor(t api.Task, desc string) []string {
	return itemTexts(m.taskItemsFor(t, desc, nil))
}

// taskItemsFor builds the fields, the agent/PR/parent/url cross-references, then desc and comments.
func (m model) taskItemsFor(t api.Task, desc string, comments []api.Comment) []metaItem {
	assignee, pr := "", ""
	for _, a := range m.state.Agents {
		if a.Task == t.ID {
			assignee = a.Name
		}
	}
	for _, p := range m.state.PRs {
		if p.Task == t.ID && p.Status != "merged" {
			pr = p.ID
		}
	}
	xref := func(label, val, kind string) metaItem {
		if val == "" {
			return metaItem{text: label + "-"}
		}
		return metaItem{text: label + val, kind: kind, value: val}
	}
	items := []metaItem{
		{text: t.Title}, {text: ""},
		{text: "type:     " + dash(t.Type)},
		{text: "priority: " + theme.PriorityLabel(t.Priority)},
		{text: "status:   " + t.Status},
	}
	// The exact moments, in local time — the list column rounds them, and rounding is what a
	// question about one particular task is asking past. "changed" is the field the active filter
	// reads, so an "n/a" here explains why a mirrored task that just closed is missing from it.
	items = append(items,
		metaItem{text: "created:  " + theme.When(t.CreatedAt)},
		metaItem{text: "changed:  " + theme.When(t.UpdatedAt)},
	)
	if t.Approval != "" { // a planner proposal under the approval gate
		line := "approval: " + theme.ApprovalLabel(t.Approval)
		if t.ApprovalComment != "" {
			line += " — " + t.ApprovalComment
		}
		items = append(items, metaItem{text: line})
	}
	items = append(items,
		xref("parent:   ", t.ParentID, "task"),
		xref("agent:    ", assignee, "agent"),
		xref("pr:       ", pr, "pr"),
		xref("url:      ", t.URL, "url"), // e.g. the GitHub issue; enter copies it (onkey.go)
		metaItem{text: "labels:   " + dash(t.Labels)},
	)
	items = append(items, descItems(desc)...)
	return append(items, commentItems(comments)...)
}

// descItems renders an optional description block.
func descItems(desc string) []metaItem {
	if strings.TrimSpace(desc) == "" {
		return nil
	}
	items := []metaItem{{text: ""}, {text: "── description ──"}}
	for _, l := range strings.Split(strings.TrimRight(desc, "\n"), "\n") {
		items = append(items, metaItem{text: l})
	}
	return items
}

// commentItems renders the synced thread as author + local timestamp, then body lines.
func commentItems(comments []api.Comment) []metaItem {
	if len(comments) == 0 {
		return nil
	}
	items := []metaItem{{text: ""}, {text: fmt.Sprintf("── comments (%d) ──", len(comments))}}
	for _, c := range comments {
		// The source too: "github" means the comment came from or went to the upstream issue, so
		// it says who else has already seen it — which a reply is written differently for.
		head := c.Author + " (" + c.Source + ")"
		if ts := commentTime(c.CreatedAt); ts != "" {
			head = ts + "  " + head
		}
		items = append(items, metaItem{text: ""}, metaItem{text: dimStyle.Render(head)})
		for _, l := range strings.Split(strings.TrimRight(c.Body, "\n"), "\n") {
			items = append(items, metaItem{text: l})
		}
	}
	return items
}

// commentTime formats local date + time ("" if unparseable); threads span days, so HH:MM won't do.
func commentTime(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Local().Format("2006-01-02 15:04")
	}
	return ""
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
	title, typ, prio, parent, labels, desc, id := "", "task", "P2", "", "", "", ""
	if edit {
		id, title, prio, parent, labels, desc = t.ID, t.Title, t.Priority, t.ParentID, t.Labels, t.Description
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
	m.form.open(heading, []field{titleF, typeF, prioF, parentF, labelsF, descF}, validate, func() tea.Cmd {
		spec := api.TaskSpec{
			Title: titleF.value(), Type: typeF.value(), Priority: prioF.value(),
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

// whyNextCmd asks the hub what it would assign next and why nothing else, shown as a notice — the
// same account `sindri task next` prints, since the reasoning is the hub's and neither front-end
// gets to have its own version of it.
func (m *model) whyNextCmd() tea.Cmd {
	cl := m.cl
	if cl == nil {
		return nil
	}
	return func() tea.Msg {
		x, err := cl.NextTask("")
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
