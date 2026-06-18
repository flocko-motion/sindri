// package: tui / tasks
// type:    ui (Tasks tab)
// job:     the Tasks tab content — the hierarchical tree selector (filtered
//          open/closed/all, collapsible, PR-marked) and the task detail pane.
//          Tree arrangement + PR annotation come from the hub (ArrangeTasks);
//          this renders rows and folds.
package tui

import (
	"fmt"
	"strings"

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

		mark := " "
		if tr.PR != "" {
			mark = "◆"
		}
		out[i] = row{
			fmt.Sprintf("%s %-9s %-5s %-8s %-6s %s %s",
				gutter, tr.ID, typeAbbr(tr.Type), hub.PriorityLabel(tr.Priority), hub.StateLabel(tr.Status), mark, tr.Title),
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
	id := m.selID()
	if id == "" {
		return []string{dimStyle.Render("(no task)")}
	}
	var t store.Task
	for _, x := range m.state.Tasks {
		if x.ID == id {
			t = x
		}
	}
	assignee := "-"
	for _, a := range m.state.Agents {
		if a.Task == id {
			assignee = a.Name
		}
	}
	pr := "-"
	for _, p := range m.state.PRs {
		if p.Task == id && p.Status != "merged" {
			pr = p.ID
		}
	}
	ls := []string{
		t.Title, "",
		"type:     " + dash(t.Type),
		"priority: " + hub.PriorityLabel(t.Priority),
		"status:   " + t.Status,
		"parent:   " + dash(t.ParentID),
		"agent:    " + assignee,
		"pr:       " + pr,
		"labels:   " + dash(t.Labels),
	}
	if m.taskDetail.ID == id && strings.TrimSpace(m.taskDetail.Description) != "" {
		ls = append(ls, "", "── description ──")
		ls = append(ls, strings.Split(strings.TrimRight(m.taskDetail.Description, "\n"), "\n")...)
	}
	return ls
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
		if strings.TrimSpace(titleF.value()) == "" {
			return "title can't be empty"
		}
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
			if edit {
				_ = cl.EditTask(id, spec)
			} else if strings.TrimSpace(spec.Title) != "" {
				_, _ = cl.CreateTask(spec)
			}
			return nil
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
