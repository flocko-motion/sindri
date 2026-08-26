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
	m.focus = focusList
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

// wrappedDetail wraps the detail pane to column width (long titles scroll rather than truncate),
// remapping the highlight index through the wrap (-1 for none).
// reclamp already wrapped this frame's content before View runs, so this is normally a cache hit —
// nil for calls, since a genuine miss here is a fallback this method cannot make reclamp reuse.
func (m model) wrappedDetail() (lines []string, highlight int) {
	c := detailWrap(m.detailWrapCache, m.tab, m.detailWidth(), m.detailLines(), nil)
	if h := m.detailHighlight(); h >= 0 && h < len(c.origAt) {
		return c.wrapped, c.origAt[h]
	}
	return c.wrapped, -1
}

// focusedDetailLine is the wrapped-line index — matching scrollTarget()'s coordinate space — of
// the item rightCursor currently focuses, or -1. The same index prMetaLines/agentMetaLines/
// wrappedDetail already compute for rendering the highlight, reused here so scrolling can reveal it.
func (m model) focusedDetailLine() int {
	if m.focus != focusItems {
		return -1
	}
	switch m.tab {
	case 2:
		_, hl := m.prMetaLines(max(1, m.w-m.prContentWidth()-1))
		return hl
	case 1:
		_, hl := m.agentMetaLines(m.agentDetailWidth())
		return hl
	default:
		_, hl := m.wrappedDetail()
		return hl
	}
}

// revealFocusedItem scrolls scrollTarget() so the item rightCursor currently focuses sits inside
// its window. Called wherever rightCursor changes — ctrl+l entering focusItems, and j/k stepping
// within it — so the highlight is never left off screen (sd-57e895 round 5: ctrl+l used to clamp
// rightCursor without ever scrolling to it, and j/k inherited that gap from the other side).
func (m *model) revealFocusedItem() {
	if line := m.focusedDetailLine(); line >= 0 {
		m.scrollTarget().SetCursor(line)
	}
}

// scrollFold moves scrollTarget() one line, folding the REAL movement into detailExcess — Scroll*
// clamps silently at the pane's ends, so counting the call itself let k/j drift (round 8).
func (m *model) scrollFold(forward bool) {
	vp := m.scrollTarget()
	before := vp.Offset
	if forward {
		vp.ScrollDown()
	} else {
		vp.ScrollUp()
	}
	switch moved := vp.Offset - before; {
	case moved != 0:
		m.detailExcess = max(0, m.detailExcess+moved)
	case forward:
		m.flash = "already at the bottom"
	default:
		m.detailExcess = 0
		m.flash = "already at the top"
	}
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

// focusKey is one entry in a per-focus-state override table (-> rightFocusKeys, detailFocusKeys):
// the keys it relabels and what they mean instead, while that state holds.
type focusKey struct{ keys, label string }

// rightFocusKeys is what j/k, enter, g and y mean while a detail/meta column's ITEMS have focus —
// overriding every tab's ordinary bindings for those same letters. One table, so the footer's
// hint and "?"'s reference (helpLines) cannot describe two different focused worlds.
var rightFocusKeys = []focusKey{
	{"j/k", "item"},
	{keyEnter, "details"},
	{"g", "goto"},
	{"G", "bottom"}, // past the last item, into whatever the cursor cannot reach (review round 6)
	{"y", "copy"},
}

// detailFocusKeys is j/k/g/G's meaning while a pane's raw CONTENT has focus (focusDetail) — the
// same one-table discipline rightFocusKeys already holds focusItems to, so this state cannot
// drift between the footer's hint and "?"'s reference either (review round 5).
var detailFocusKeys = []focusKey{
	{"j/k", "scroll"},
	{"g/G", "top/bot"},
}

// footerFromTable renders a focus override table as one footer-style line.
func footerFromTable(table []focusKey) string {
	parts := make([]string, len(table))
	for i, r := range table {
		parts[i] = r.keys + " " + r.label
	}
	return strings.Join(parts, " · ")
}

// contextFooter is the tab's action hints, generated from the keymap so help can't drift.
func (m model) contextFooter() string {
	switch m.focus {
	case focusItems: // focused on a detail cross-reference
		return footerFromTable(rightFocusKeys)
	case focusDetail: // focused on the pane's raw content
		return footerFromTable(detailFocusKeys)
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

// focusedItem is the right-column item the cursor is on (when focus is focusItems).
func (m model) focusedItem() (metaItem, bool) {
	act := m.actionableItems()
	if m.rightCursor >= 0 && m.rightCursor < len(act) {
		return act[m.rightCursor], true
	}
	return metaItem{}, false
}

// idsNeedingUser is the ids, on the active tab, of rows that need the user — read off the exact
// predicate that already marks that row (api.TaskNeedsUser, api.AgentNeedsUser, api.PRNeedsUser —
// each also feeding its section's badge — and Mail's to-you-and-unread row marker, whose badge
// parity is sd-ac1757's to land). Never a second definition, or the row and the key would disagree
// about what needs attention. nil on a tab with no such notion.
func (m model) idsNeedingUser() map[string]bool {
	out := map[string]bool{}
	switch m.tab {
	case 0:
		released := api.ReleasedByPriority(m.state.Tasks)
		for _, t := range m.state.Tasks {
			if api.TaskNeedsUser(t, released[t.ID]) {
				out[t.ID] = true
			}
		}
	case 1:
		for _, a := range m.state.Agents {
			if api.AgentNeedsUser(a) {
				out[a.Name] = true
			}
		}
	case 2:
		for _, p := range m.state.PRs {
			if api.PRNeedsUser(p, m.state.Agents) {
				out[p.ID] = true
			}
		}
	case 6:
		for _, msg := range m.state.Mail {
			if api.MailToUser(msg) && !msg.Read() {
				out[api.MailID(msg.ID)] = true
			}
		}
	default:
		return nil
	}
	return out
}

// moveToNeedingUser moves the cursor to the next (dir>0) or previous (dir<0) VISIBLE row that needs
// the user (-> idsNeedingUser) — visible meaning on screen right now: a match hidden by the active
// filter or folded under a collapsed Tasks parent is not a candidate, since m.rows() already leaves
// those out. Never cycles: past the last match it flashes rather than wrapping, since a key that
// silently does nothing reads as broken. A tab with no such notion does nothing at all.
func (m *model) moveToNeedingUser(dir int) {
	needs := m.idsNeedingUser()
	if needs == nil {
		return
	}
	rows := m.rows()
	cur := m.selRow()
	for i := cur + dir; i >= 0 && i < len(rows); i += dir {
		if r := rows[i]; r.selectable() && needs[r.id] {
			m.cursor[m.tab] = i
			return
		}
	}
	if dir > 0 {
		m.flash = "no later row needs you"
	} else {
		m.flash = "no earlier row needs you"
	}
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
	if m.focus != focusItems {
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
