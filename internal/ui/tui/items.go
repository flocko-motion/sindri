// package: tui / items
// type:    ui (cross-reference navigation)
// job:     the global item convention — ENTER opens an item's details in the
// big modal (using that item's own home-tab renderer), and g goes to
// where the item lives (its home tab, selected). Shared by the
// focusable detail columns on every tab.
// limits:  routing and opening only; each item's detail rendering is its home
// tab's (-> tab_*.go).
package tui

import (
	"strings"

	"github.com/flo-at/sindri/internal/api"
)

// detailLines is the current tab's detail content (the right column / modal body).
func (m model) detailLines() []string {
	switch m.tab {
	case 0:
		return m.taskDetailLines()
	case 1:
		return m.agentDetailLines()
	case 2:
		return m.prDetailLines()
	default:
		return m.repoDetailLines()
	}
}

// modalTitle titles the full-screen detail modal by the active tab's item.
func (m model) modalTitle() string {
	switch m.tab {
	case 0:
		return "Task " + m.selID()
	case 1:
		return "Agent " + m.selID()
	case 2:
		return "PR " + m.selID()
	default:
		return "Repo " + m.repoName(m.selID())
	}
}

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

// itemDetailLines renders via the item's home-tab renderer, so ENTER-peek matches its own view.
func (m model) itemDetailLines(kind, id string) []string {
	switch kind {
	case "task":
		for _, t := range m.state.Tasks {
			if t.ID == id {
				desc := t.Description // board row carries it — shown at once
				if m.taskDetail.ID == id && m.taskDetail.Description != "" {
					desc = m.taskDetail.Description // refined by the detail read
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

// prMetaFromPR is the basic PR detail from board state — no diff/reviews fetched yet.
func prMetaFromPR(p api.PR) []string {
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

// openTextModal shows arbitrary command output — a build log, a stats table — in the same scrollable
// modal the item details use, so long output is readable rather than a flash that scrolls past.
func (m *model) openTextModal(title, body string) {
	m.modalOverride = strings.Split(strings.TrimRight(body, "\n"), "\n")
	m.modalOverrideTitle = title
	m.modal = true
	m.detail.SetHeight(modalContentHeight(m.h))
	m.detail.SetTotal(len(m.modalLines()))
	m.detail.ScrollTop()
}

// openItemModal opens the big detail modal for any item, via its home renderer.
func (m *model) openItemModal(kind, id string) {
	m.modalOverride = m.itemDetailLines(kind, id)
	m.modalOverrideTitle = m.itemTitle(kind, id)
	m.modal = true
	m.detail.SetHeight(modalContentHeight(m.h))
	m.detail.SetTotal(len(m.modalLines())) // wrapped count, matching the render
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

// selID is the id of the row under the active tab's cursor ("" if none).
func (m model) selID() string {
	r := m.rows()
	if c := m.cursor[m.tab]; c >= 0 && c < len(r) {
		return r[c].id
	}
	return ""
}

// wrappedDetail wraps the detail pane to column width (long titles scroll with J/K
// rather than truncate), remapping the highlight index through the wrap (-1 for none).
func (m model) wrappedDetail() (lines []string, highlight int) {
	wrapped, origAt := wrapContentMapped(m.detailLines(), m.detailWidth())
	if h := m.detailHighlight(); h >= 0 && h < len(origAt) {
		return wrapped, origAt[h]
	}
	return wrapped, -1
}

// rows dispatches to the active tab's row builder (tasks/agents/prs).
func (m model) rows() []row {
	switch m.tab {
	case 0:
		return m.taskRows()
	case 1:
		return m.agentRows()
	case 2:
		return m.prRows()
	case 3:
		return m.repoRows()
	default:
		return nil // Chat has no selectable rows — it renders its own transcript body
	}
}

// inScope admits a board item under the active scope. Single home of that rule, so tab
// badges can't drift from the lists beneath them.
func (m model) inScope(project string) bool {
	if !m.scopeRepo {
		return true
	}
	_, tag := m.currentRepo()
	if tag == "" {
		// No nameable repo (registry hiccup, unregistered cwd): scoping would blank every
		// row, so show everything — a wider view beats an empty one.
		return true
	}
	return project == tag
}

// agentVisible admits an agent to the Agents tab: in scope, or waiting on the user anywhere in the
// fleet. The attention marker beside the handle is fleet-wide by design (-> View), so a scope that
// hid the row it points at left the user told that something needs them and shown a list where
// nothing does. The row carries its own repo in the first column, which is what says it is foreign.
func (m model) agentVisible(a api.AgentView) bool {
	return m.inScope(a.Project) || api.AgentNeedsUser(a)
}

// tabCount is section s's badge. Agents/PRs obey the § scope toggle so the badge matches
// the list; the rest are scope-invariant and read straight off the board.
func (m model) tabCount(s tuiSection) int {
	switch s.Key {
	case "agents":
		n := 0
		for _, a := range m.state.Agents {
			if m.agentVisible(a) {
				n++
			}
		}
		return n
	case "prs":
		n := 0
		for _, p := range m.state.PRs {
			if m.inScope(p.Project) && api.PROpen(p) {
				n++
			}
		}
		return n
	case "tasks":
		return m.state.OpenTaskCount()
	case "repos":
		return m.state.RepoCount()
	case "chat":
		return m.state.ChatMemberCount()
	}
	return 0
}

// scopeName labels the global↔repo scope toggle for the footer.
func scopeName(repoScoped bool) string {
	if repoScoped {
		return "repo"
	}
	return "global"
}

// contextFooter is the tab's action hints, generated from the keymap so help can't drift.
func (m model) contextFooter() string {
	if m.rightFocus { // focused on a detail cross-reference (Tasks/PRs)
		return "j/k item · enter details · g goto · y copy"
	}
	return m.footerFor(tabScope(m.tab))
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

// detailHighlight is the Tasks detail line to highlight, or -1. (PRs highlight in prBody.)
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
