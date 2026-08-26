// package: tui / task detail
// type:    ui (Tasks tab — the detail pane's items)
// job:     a task's detail as focusable items: fields, then the parent/children/agent/pr/url
// cross-references, then description and comments. What the right column renders,
// `Y` copies, and other tabs borrow when they point at a task (-> taskDetailFor).
// limits:  rendering only; the fields come off the board (-> tab_tasks.go for rows/tree/actions).
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/theme"
)

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
	// One rule for who is behind the task, shared with the row marker and the CLI: a live claim,
	// else the author of the PR under review — a submitted task still has an owner, and that is
	// the reader's question when they open one that is waiting on a verdict.
	holder, pr := api.AgentOnTask(m.state.Agents, m.state.PRs, t.ID), ""
	for _, p := range m.state.PRs {
		if p.Task == t.ID && p.Status != "merged" {
			pr = p.ID
		}
	}
	// agentXref names the relationship beside the agent, never just the name: an agent holding a
	// feature and working a leaf inside it appears against BOTH rows, and two identical lines read as
	// two agents in one tree. The value stays the bare name, since that is what ENTER jumps to.
	agentXref := func(h api.TaskHolder) metaItem {
		if h.Agent == "" {
			return metaItem{text: "agent:    -"}
		}
		return metaItem{text: "agent:    " + h.Agent + " — " + theme.TaskRelationLabel(h.Rel), kind: "agent", value: h.Agent}
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
		{text: "tier:     " + api.TierOrDefault(t.Tier)},
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
	items = append(items, xref("parent:   ", t.ParentID, "task"))
	items = append(items, childItems(m.state.Tasks, t.ID)...)
	items = append(items,
		agentXref(holder),
		xref("pr:       ", pr, "pr"),
		xref("url:      ", t.URL, "url"), // e.g. the GitHub issue; enter copies it (onkey.go)
		metaItem{text: "labels:   " + dash(t.Labels)},
	)
	items = append(items, descItems(desc)...)
	return append(items, commentItems(comments)...)
}

// childItems lists a task's direct children as cross-references, "children: -" for none — the
// parent xref's counterpart, so the tree walks down as readily as it walks up.
func childItems(tasks []api.Task, parentID string) []metaItem {
	var kids []api.Task
	for _, t := range tasks {
		if t.ParentID == parentID {
			kids = append(kids, t)
		}
	}
	if len(kids) == 0 {
		return []metaItem{{text: "children: -"}}
	}
	items := make([]metaItem, len(kids))
	for i, k := range kids {
		label := "children: "
		if i > 0 {
			label = "          "
		}
		items[i] = metaItem{text: label + k.ID + " " + k.Title, kind: "task", value: k.ID}
	}
	return items
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
