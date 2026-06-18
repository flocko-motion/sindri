// package: tui / items
// type:    ui (cross-reference navigation)
// job:     the global item convention — ENTER opens an item's details in the
//          big modal (using that item's own home-tab renderer), and g goes to
//          where the item lives (its home tab, selected). Shared by the
//          focusable detail columns on every tab.
package tui

import "github.com/flo-at/sindri/internal/hub/store"

// homeTab maps an item kind to the tab where it lives (-1 if none).
func homeTab(kind string) int {
	switch kind {
	case "task":
		return 0
	case "agent":
		return 1
	case "pr":
		return 2
	}
	return -1
}

// itemTexts is the plain text of a metaItem slice (for the modal / pane body).
func itemTexts(items []metaItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.text
	}
	return out
}

// itemDetailLines renders an item's detail with its home-tab renderer, so the
// modal-peek (ENTER) is identical to the item's own detail view.
func (m model) itemDetailLines(kind, id string) []string {
	switch kind {
	case "task":
		for _, t := range m.state.Tasks {
			if t.ID == id {
				desc := ""
				if m.taskDetail.ID == id {
					desc = m.taskDetail.Description
				}
				return m.taskDetailFor(t, desc)
			}
		}
	case "agent":
		for _, a := range m.state.Agents {
			if a.Name == id {
				return m.agentDetailFor(a)
			}
		}
	case "pr":
		if m.prDetail.PR.ID == id { // the fetched, rich detail
			return m.prDetailLines()
		}
		for _, p := range m.state.PRs {
			if p.ID == id {
				return prMetaFromPR(p)
			}
		}
	}
	return []string{dimStyle.Render("(not found: " + id + ")")}
}

func (m model) itemTitle(kind, id string) string {
	switch kind {
	case "task":
		return "Task " + id
	case "agent":
		return "Agent " + id
	case "pr":
		return "PR " + id
	}
	return id
}

// prMetaFromPR is the basic PR detail from board state (when it isn't the
// fetched selection, so there's no diff/reviews yet).
func prMetaFromPR(p store.PR) []string {
	ls := []string{
		p.ID,
		"status: " + p.Status,
		"agent:  " + p.Agent,
		"branch: " + p.Branch + " → " + p.Base,
		"task:   " + p.Task,
	}
	if p.Feedback != "" {
		ls = append(ls, "feedback: "+p.Feedback)
	}
	return ls
}

// openItemModal opens the big detail modal for any item, via its home renderer.
func (m *model) openItemModal(kind, id string) {
	m.modalOverride = m.itemDetailLines(kind, id)
	m.modalOverrideTitle = m.itemTitle(kind, id)
	m.modal = true
	m.detail.SetHeight(modalContentHeight(m.h))
	m.detail.SetTotal(len(m.modalOverride))
	m.detail.ScrollTop()
}

// selectRow moves the current tab's cursor to the row with the given id.
func (m *model) selectRow(id string) {
	for i, r := range m.rows() {
		if r.id == id {
			m.cursor[m.tab] = i
			return
		}
	}
}

// gotoItem navigates to where an item lives: its home tab, with it selected.
func (m *model) gotoItem(kind, id string) {
	t := homeTab(kind)
	if t < 0 {
		return
	}
	m.rightFocus = false
	m.tab = t
	m.selectRow(id)
}

// actionableItems is the focusable cross-references of the current tab's detail.
func (m model) actionableItems() []metaItem {
	switch m.tab {
	case 0:
		return m.taskActionable()
	case 1:
		return m.agentActionable()
	case 2:
		return m.prActionable()
	}
	return nil
}

// focusedItem is the right-column item the cursor is on (when right-focused).
func (m model) focusedItem() (metaItem, bool) {
	act := m.actionableItems()
	if m.rightCursor >= 0 && m.rightCursor < len(act) {
		return act[m.rightCursor], true
	}
	return metaItem{}, false
}

// detailHighlight is the line index in the Tasks detail pane to highlight (the
// focused cross-reference when right-focused), or -1. (PRs highlight in prBody.)
func (m model) detailHighlight() int {
	if !m.rightFocus || m.tab != 0 {
		return -1
	}
	ai := 0
	for i, it := range m.taskItems() {
		if it.kind != "" {
			if ai == m.rightCursor {
				return i
			}
			ai++
		}
	}
	return -1
}
