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

	var out []row
	hideAbove := -1 // depth of a collapsed ancestor; rows deeper than this are hidden
	for _, tr := range arranged {
		if hideAbove >= 0 && tr.Depth > hideAbove {
			continue
		}
		hideAbove = -1
		fold := " "
		if hasKids[tr.ID] {
			if m.collapsed[tr.ID] {
				fold = "▸"
			} else {
				fold = "▾"
			}
		}
		mark := ""
		if tr.PR != "" {
			mark = "◆ "
		}
		// Fixed left columns (priority word, status) keep alignment; the tree
		// indent applies only to the title region so hierarchy never shifts them.
		title := strings.Repeat("  ", tr.Depth) + fold + " " + mark + tr.Title
		text := fmt.Sprintf("%-8s %-11s %s", hub.PriorityLabel(tr.Priority), tr.Status, title)
		out = append(out, row{text, tr.ID})
		if m.collapsed[tr.ID] {
			hideAbove = tr.Depth
		}
	}
	return out
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
