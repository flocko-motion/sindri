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
	case 5:
		return m.runDetailLines()
	case 6:
		return m.mailDetailLines()
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
	case 6:
		return "Message " + m.selID()
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
	case "mail":
		return 6
	}
	return -1
}

// isAgent reports whether name is a live entry on the roster — the check a sender/queuer string
// needs before it can be offered as an "agent" cross-reference, since hub/user/reviewer and a
// retired agent's old name are not reachable on the Agents tab.
func (m model) isAgent(name string) bool {
	for _, a := range m.state.Agents {
		if a.Name == name {
			return true
		}
	}
	return false
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

// openHelpModal shows "?"'s reference in the same scrollable modal every other detail view uses.
func (m *model) openHelpModal() {
	m.modalOverride = m.helpLines()
	m.modalOverrideTitle = "Help" // the body's own first line already names the tab
	m.modal = true
	m.detail.SetHeight(modalContentHeight(m.h))
	m.detail.SetTotal(len(m.modalLines()))
	m.detail.ScrollTop()
}

// selectRow moves the current tab's cursor to the row with the given id, reporting whether it
// found one — a caller that must land somewhere DEFINITE when it did not (the first row, never
// wherever reclamp's index-clamp happened to leave the cursor) needs to tell the two apart.
func (m *model) selectRow(id string) bool {
	for i, r := range m.rows() {
		if r.id == id {
			m.cursor[m.tab] = i
			return true
		}
	}
	return false
}

// restoreSelection re-filters, then puts the cursor back on sel if a re-filter left it visible —
// the first row otherwise, never wherever reclamp's plain index-clamp happened to land, which is
// a different row entirely once enough rows above the old index have dropped out.
func (m *model) restoreSelection(sel string) {
	m.reclamp()
	if !m.selectRow(sel) {
		m.cursor[m.tab] = 0
		m.reclamp()
	}
}

// gotoItem navigates to where an item lives: its home tab, with it selected — widened first if
// the destination's own filter hides it, or the jump lands beside it (or not at all).
func (m *model) gotoItem(kind, id string) {
	t := homeTab(kind)
	if t < 0 {
		return
	}
	m.rightFocus = false
	m.tab = t
	// Mail is reached by NARROWING rather than by selecting: the item is a count of an agent's
	// unread messages, not one row, so what it names is a set (-> showUnreadFor).
	if kind == "mail" {
		m.showUnreadFor(id)
		return
	}
	if m.selectRow(id) {
		return
	}
	if w := m.widenTabFor(t); w != "" && m.selectRow(id) {
		m.flash = id + " was hidden by " + w + " — widened to show it"
		return
	}
	m.flash = "can't find " + id + " on the board"
}

// widenTabFor relaxes every axis that could hide one row on tab t (Tasks: filter and search;
// PRs: filter and scope; Agents: scope), reporting what it widened ("" if already widest).
func (m *model) widenTabFor(t int) string {
	var widened []string
	switch t {
	case 0:
		if m.filter != api.FilterAll {
			m.filter = api.FilterAll
			widened = append(widened, "the task filter")
		}
		if m.taskSearch != "" {
			m.taskSearch = ""
			widened = append(widened, "the search")
		}
	case 1:
		if m.scopeRepo {
			m.scopeRepo = false
			widened = append(widened, "the repo scope")
		}
	case 2:
		if m.prFilter != api.PRFilterAll {
			m.prFilter = api.PRFilterAll
			widened = append(widened, "the PR filter")
		}
		if m.scopeRepo {
			m.scopeRepo = false
			widened = append(widened, "the repo scope")
		}
	}
	return strings.Join(widened, " and ")
}

// moveCursor moves the active tab's cursor by delta rows and leaves it on a row that selects
// something. Every key that moves the selection goes through it, so none of them can land on a
// heading and leave the detail pane with nothing to show.
func (m *model) moveCursor(delta int) {
	rows := m.rows()
	if len(rows) == 0 {
		m.cursor[m.tab] = 0
		return
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	m.cursor[m.tab] = nearestSelectable(rows, clampInt(m.cursor[m.tab]+delta, 0, len(rows)-1), step)
}

// nearestSelectable is the first row from i that selects something, searched in step's direction
// then back the other way — a heading sits above its rows, a spacer below, either way out.
func nearestSelectable(rows []row, i, step int) int {
	for j := i; j >= 0 && j < len(rows); j += step {
		if rows[j].selectable() {
			return j
		}
	}
	for j := i; j >= 0 && j < len(rows); j -= step {
		if rows[j].selectable() {
			return j
		}
	}
	return i
}

// selRow is the row the cursor selects: the stored index, snapped forward onto a row that selects
// something — a model not yet laid out has its cursor at 0, where a labelled list keeps its labels.
func (m model) selRow() int {
	rows := m.rows()
	if len(rows) == 0 {
		return 0
	}
	return nearestSelectable(rows, clampInt(m.cursor[m.tab], 0, len(rows)-1), 1)
}

// selID is the id of the row under the active tab's cursor ("" if none).
func (m model) selID() string {
	r := m.rows()
	if c := m.selRow(); c >= 0 && c < len(r) {
		return r[c].id
	}
	return ""
}

// wrappedDetail wraps the detail pane to column width (long titles scroll with J/K
// rather than truncate), remapping the highlight index through the wrap (-1 for none).
// reclamp already wrapped this frame's content before View runs, so this is normally a cache hit —
// nil for calls, since a genuine miss here is a fallback this method cannot make reclamp reuse.
func (m model) wrappedDetail() (lines []string, highlight int) {
	c := detailWrap(m.detailWrapCache, m.tab, m.detailWidth(), m.detailLines(), nil)
	if h := m.detailHighlight(); h >= 0 && h < len(c.origAt) {
		return c.wrapped, c.origAt[h]
	}
	return c.wrapped, -1
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
	case 5:
		return m.runRows()
	case 6:
		return m.mailRows()
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
	if project == api.GlobalProject {
		return true // belongs to no repo, so it is never foreign to one
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
// fleet — the attention marker beside the handle is fleet-wide by design (-> View).
func (m model) agentVisible(a api.AgentView) bool {
	return m.inScope(a.Project) || api.AgentNeedsUser(a)
}

// prVisible admits a PR to the PRs tab, on the same rule and reason as agentVisible: background
// work waiting on the user has to surface wherever they are (-> api.PRNeedsUser).
func (m model) prVisible(p api.PR) bool {
	return m.inScope(p.Project) || api.PRNeedsUser(p, m.state.Agents)
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
			if m.prVisible(p) && api.PROpen(p) {
				n++
			}
		}
		return n
	case "runs":
		n := 0
		for _, r := range m.state.Runs {
			if m.inScope(r.Project) && api.RunOpen(r) {
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
	case "mail":
		// Unread over the whole mailbox, not the rendered window, or the badge would go quiet
		// exactly when there was most unread. Narrowed by § like Agents/PRs, from the per-repo tally.
		if _, tag := m.currentRepo(); m.scopeRepo && tag != "" {
			return m.state.MailUnreadByRepo[tag]
		}
		return m.state.MailUnread
	}
	return 0
}

// scopeNeedsYou reports whether the active tab's repo scope keeps anything waiting on the user
// from any repo (Agents, PRs, Mail — agentVisible, prVisible, mailVisible) or is the repo alone
// (Runs — runRows filters on inScope alone). Read off the tab rather than passed in at each
// scopeName call site, so which tabs keep it cannot be gotten wrong there.
func scopeNeedsYou(scope keyScope) bool {
	switch scope {
	case scopeAgents, scopePRs, scopeMail:
		return true
	default:
		return false
	}
}

// scopeName labels the global↔repo scope toggle: named for what it does, or it promises a row the
// scope does not keep (a label naming the repo alone would claim to exclude what it plainly shows).
func scopeName(repoScoped bool, m model) string {
	if !repoScoped {
		return "global"
	}
	if scopeNeedsYou(tabScope(m.tab)) {
		return "repo+needs-you"
	}
	return "repo"
}

// rightFocusKeys is what j/k, enter, g and y mean while the detail/meta column has focus —
// overriding every tab's ordinary bindings for those same letters. One table, so the footer's
// hint and "?"'s reference (helpLines) cannot describe two different focused worlds.
var rightFocusKeys = []struct{ keys, label string }{
	{"j/k", "item"},
	{keyEnter, "details"},
	{"g", "goto"},
	{"y", "copy"},
}

// contextFooter is the tab's action hints, generated from the keymap so help can't drift.
func (m model) contextFooter() string {
	if m.rightFocus { // focused on a detail cross-reference
		var parts []string
		for _, r := range rightFocusKeys {
			parts = append(parts, r.keys+" "+r.label)
		}
		return strings.Join(parts, " · ")
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
	case 3:
		return m.repoActionable()
	case 5:
		return m.runActionable()
	case 6:
		return m.mailActionable()
	}
	return nil // Chat (4) has no detail pane — it renders its own transcript body
}

// focusedItem is the right-column item the cursor is on (when right-focused).
func (m model) focusedItem() (metaItem, bool) {
	act := m.actionableItems()
	if m.rightCursor >= 0 && m.rightCursor < len(act) {
		return act[m.rightCursor], true
	}
	return metaItem{}, false
}

// unwrappedDetailItems is the raw (pre-wrap) items behind a tab's detailLines(), for tabs whose
// detail renders through the generic pane path (Agents/PRs/Mail wrap or highlight their own way).
func (m model) unwrappedDetailItems() []metaItem {
	switch m.tab {
	case 0:
		return m.taskItems()
	case 3:
		return m.repoItems()
	case 5:
		return m.runItems()
	}
	return nil
}

// detailHighlight is the detail line to highlight, or -1.
func (m model) detailHighlight() int {
	if !m.rightFocus {
		return -1
	}
	items := m.unwrappedDetailItems()
	if items == nil {
		return -1
	}
	ai := 0
	for i, it := range items {
		if it.kind != "" {
			if ai == m.rightCursor {
				return i
			}
			ai++
		}
	}
	return -1
}
