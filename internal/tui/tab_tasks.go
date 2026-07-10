// package: tui / tasks
// type:    ui (Tasks tab)
// job:     the Tasks tab content — the hierarchical tree selector (filtered
//          open/closed/all, collapsible, PR-marked) and the task detail pane.
//          Tree arrangement + PR annotation come from the hub (ArrangeTasks);
//          this renders rows and folds.
// limits:  renders rows and folds only; tree arrangement + PR annotation are the
//          hub's (-> ArrangeTasks).
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

func isDone(status string) bool {
	switch status {
	case "closed", "approved", "merged":
		return true
	}
	return false
}

// taskRows builds the filtered, folded, depth-indented task tree.
func (m model) taskRows() []row {
	var filtered []store.Task
	for _, t := range m.state.Tasks {
		done := isDone(t.Status)
		if m.filter == filterAll || (m.filter == filterOpen && !done) || (m.filter == filterClosed && done) {
			filtered = append(filtered, t)
		}
	}
	arranged := hub.ArrangeTasks(filtered, m.state.PRs)

	// Which tasks have a worker on them right now (drives the 🔨 marker).
	assigned := map[string]bool{}
	for _, a := range m.state.Agents {
		if a.Task != "" {
			assigned[a.Task] = true
		}
	}
	// Hub-side approval per task (drives the row colour for planner proposals).
	approval := map[string]string{}
	for _, t := range m.state.Tasks {
		if t.Approval != "" {
			approval[t.ID] = t.Approval
		}
	}

	// A node has children if a later row is exactly one level deeper before the
	// depth returns to its level.
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
	var visible []hub.TaskRow
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

	// Columns: fixed tree gutter (│ ├ └ connectors) · id · type · prio · state ·
	// title. The gutter is fixed-width so id and the rest stay aligned; the tree
	// lives entirely in the gutter.
	out := make([]row, len(visible))
	cont := []bool{} // cont[i]: ancestor at depth i has a later sibling (draw │)
	for i, tr := range visible {
		if len(cont) > tr.Depth {
			cont = cont[:tr.Depth]
		}
		gutter := treeGutter(cont, tr.Depth, tr.Last, hasKids[tr.ID], m.collapsed[tr.ID])
		cont = append(cont, !tr.Last)

		// Each cell is styled independently (no nesting) so a colour reset never
		// bleeds across the row: status colour throughout, red for a critical
		// priority cell. The tree gutter stays uncoloured. A planner proposal under
		// the approval gate overrides the colour — yellow pending, grey rejected.
		sc := taskStatusStyle(tr.Status)
		state := hub.StateLabel(tr.Status)
		switch approval[tr.ID] { // the approval gate overrides both colour and state word
		case "pending":
			sc, state = stWarn, "pending"
		case "rejected":
			sc, state = stDone, "rejected"
		}
		prio := sc.Render(fmt.Sprintf("%-8s", hub.PriorityLabel(tr.Priority)))
		if isCriticalPriority(tr.Priority) {
			prio = stCrit.Render(fmt.Sprintf("%-8s", hub.PriorityLabel(tr.Priority)))
		}
		out[i] = row{
			strings.Join([]string{
				gutter,
				sc.Render(fmt.Sprintf("%-9s", tr.ID)),
				sc.Render(fmt.Sprintf("%-5s", typeAbbr(tr.Type))),
				prio,
				sc.Render(fmt.Sprintf("%-8s", state)),
				sc.Render(taskMarks(assigned[tr.ID], tr.PR != "")),
				sc.Render(tr.Title),
			}, " "),
			tr.ID,
		}
	}
	return out
}

const treeGutterW = 6 // fits ~3 levels of "│ "/"├─" connectors

// treeGutter draws the fixed-width tree connectors for a node: ancestor pipes,
// the branch into this node, and a fold marker for collapsible nodes.
func treeGutter(cont []bool, depth int, last, kids, collapsed bool) string {
	var b strings.Builder
	for i := 0; i < depth; i++ {
		switch {
		case i < depth-1:
			// pipe if the path node at depth i+1 is a non-last child (its parent's
			// child-list continues below this row)
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

// marksW is the display width of the status-marker column. 🔨 is two cells wide,
// so the field is padded to a fixed width to keep the title column aligned.
const marksW = 3

// taskMarks is the status-marker column: 🔨 when a worker is on the task
// (dwarves at work), ◆ when it has an open PR (merge intent). Padded (ANSI/width
// aware) to a fixed width so rows line up regardless of which marks are present.
func taskMarks(assigned, hasPR bool) string {
	s := ""
	if assigned {
		s += "🔨"
	}
	if hasPR {
		s += "◆"
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

// taskDetailLines renders the selected task: board fields + assignee + PR, plus
// the description once the lazy detail fetch has filled it.
func (m model) taskDetailLines() []string {
	if m.selID() == "" {
		return []string{dimStyle.Render("(no task)")}
	}
	return itemTexts(m.taskItems())
}

// taskItems is the selected task's detail as metaItems (parent/agent/pr are
// focusable cross-references).
func (m model) taskItems() []metaItem {
	id := m.selID()
	var t store.Task
	for _, x := range m.state.Tasks {
		if x.ID == id {
			t = x
		}
	}
	desc := ""
	var comments []store.Comment
	if m.taskDetail.ID == id {
		desc = m.taskDetail.Description
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

// taskDetailFor renders any task's detail block (used for the modal-peek and the
// PRs tab's linked-task modal). desc is the (possibly empty) description.
func (m model) taskDetailFor(t store.Task, desc string) []string {
	return itemTexts(m.taskItemsFor(t, desc, nil))
}

// taskItemsFor builds a task's detail metaItems: scalar fields plus the agent,
// PR, and parent as actionable cross-references, then the description and comments.
func (m model) taskItemsFor(t store.Task, desc string, comments []store.Comment) []metaItem {
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
		{text: "priority: " + hub.PriorityLabel(t.Priority)},
		{text: "status:   " + t.Status},
	}
	if t.Approval != "" { // a planner proposal under the approval gate
		line := "approval: " + t.Approval
		if t.ApprovalComment != "" {
			line += " — " + t.ApprovalComment
		}
		items = append(items, metaItem{text: line})
	}
	items = append(items,
		xref("parent:   ", t.ParentID, "task"),
		xref("agent:    ", assignee, "agent"),
		xref("pr:       ", pr, "pr"),
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

// commentItems renders the synced comment thread (author + local timestamp, then
// the body split into lines). The detail pane word-wraps each line, so long
// comments read in full. Empty when the task has no comments.
func commentItems(comments []store.Comment) []metaItem {
	if len(comments) == 0 {
		return nil
	}
	items := []metaItem{{text: ""}, {text: fmt.Sprintf("── comments (%d) ──", len(comments))}}
	for _, c := range comments {
		head := c.Author
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

// commentTime formats a stored RFC3339 timestamp as local "2006-01-02 15:04"
// ("" if unparseable) — comments span days, so they show the date, not just HH:MM.
func commentTime(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Local().Format("2006-01-02 15:04")
	}
	return ""
}

// taskTypes is the full set of td issue types (display == td value).
var taskTypes = []string{"task", "feature", "bug", "epic", "chore"}

// selTask returns the currently-selected task from the board snapshot.
func (m model) selTask() (store.Task, bool) {
	id := m.selID()
	for _, t := range m.state.Tasks {
		if t.ID == id {
			return t, true
		}
	}
	return store.Task{}, false
}

// openTaskForm opens the new-task (edit=false) or edit-task (edit=true) form —
// the same fields either way. Edit prefills from the selected task; on a td
// task every field applies, on an openspec item only priority does (hub-side).
func (m *model) openTaskForm(edit bool) {
	prioCodes := make([]string, len(hub.PriorityWords))
	for i, w := range hub.PriorityWords {
		prioCodes[i] = hub.PriorityCode(w)
	}
	title, typ, prio, parent, labels, desc, id := "", "task", "P2", "", "", "", ""
	if edit {
		t, ok := m.selTask()
		if !ok {
			return
		}
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
	prioF := newChoiceField("priority", hub.PriorityWords, prioCodes, prio)
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
		spec := hub.TaskSpec{
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

// taskGated reports whether the selected task is a planner proposal still under
// the approval gate (pending or rejected) — the only state A/R act on.
func (m model) taskGated() bool {
	t, ok := m.selTask()
	return ok && (t.Approval == "pending" || t.Approval == "rejected")
}

// unassignTaskCmd releases the selected task back to the backlog (the hub
// refuses if a live agent is working on it — surfaced in the error modal).
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

// closeTaskCmd marks the selected task done (the hub dispatches to its backend;
// a backend that can't close surfaces the error in the modal).
func (m *model) closeTaskCmd(id string) tea.Cmd {
	cl := m.cl
	m.flash = "closing " + id + "…"
	return func() tea.Msg {
		if cl == nil {
			return nil
		}
		if err := cl.CloseTask(id); err != nil {
			return errModalMsg{err}
		}
		st, _ := cl.State()
		return polledMsg(st)
	}
}

// openScrapChoice confirms scrapping (deleting) the selected task — destructive
// (a GitHub issue delete is permanent), so it's gated behind a yes/no. The hub
// dispatches to the backend (td delete / openspec change removal / issue delete).
func (m *model) openScrapChoice(id string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true, title: "scrap " + id + "?  (discard — td delete / openspec remove / issue delete)",
		options: []string{"cancel", "scrap"}, values: []string{"cancel", "scrap"},
		apply: func(v string) tea.Cmd {
			if v != "scrap" {
				return nil
			}
			return func() tea.Msg {
				if cl == nil {
					return nil
				}
				if err := cl.DeleteTask(id); err != nil {
					return errModalMsg{err}
				}
				st, _ := cl.State()
				return polledMsg(st)
			}
		},
	}
}

// approveTaskCmd clears the approval gate on the selected task (makes it
// claimable).
func (m *model) approveTaskCmd(id string) tea.Cmd {
	cl := m.cl
	m.flash = "approving " + id + "…"
	return mutateThenRefresh(cl, func() error { return cl.ApproveTask(id) })
}

// openTaskRejectForm opens a multiline textarea to reject a proposed task with a
// comment (delivered to the planner).
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

// openPriorityChoice opens the priority picker for a task.
func (m *model) openPriorityChoice(id string) {
	cl := m.cl
	vals := make([]string, len(hub.PriorityWords))
	for i, w := range hub.PriorityWords {
		vals[i] = hub.PriorityCode(w)
	}
	m.choice = choiceModalState{
		active: true, title: "priority for " + id,
		options: hub.PriorityWords, values: vals,
		apply: func(code string) tea.Cmd {
			if cl == nil {
				return nil
			}
			return mutateThenRefresh(cl, func() error { return cl.SetPriority(id, code) })
		},
	}
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
