// package: tui / prs
// type:    ui (PRs tab)
// job:     the PRs tab content. Mirrors the Agents layout: a short PR list over
// a big content pane (diff, or lint output after L) on the left, with
// the fixed-width detail (metadata + linked task + reviews) on the
// right. Detail is lazily fetched.
// limits:  renders PR state and actions; verdict/merge logic is the hub's
// (-> client / workflow_pr.go).
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/table"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// defaultReviewPrompt pre-fills the Agentic Review instruction; the user edits it before dispatch.
const defaultReviewPrompt = "Review this PR for correctness, clarity, and fit to the task. Flag bugs, missing tests, and anything that should change."

// lintCmd asks the hub for this PR's gate result. The hub answers from its store when the commit
// already has a verdict, and otherwise queues the check — so the pane may land on either, and this
// says "asking" rather than "running": one gate runs at a time fleet-wide, and it may be a wait.
func (m *model) lintCmd(id string) tea.Cmd {
	cl := m.cl
	m.flash = "gate " + id + "…"
	m.prDetail.Lint = "asking the hub…" // shown immediately; replaced by the answer
	m.prView = "lint"
	if m.showDetail() { // the column exists to focus; otherwise the report itself is what needs it
		m.focus, m.rightCursor, m.detailExcess = focusItems, m.viewCursor("lint"), 0
		m.revealFocusedItem()
	} else {
		m.focus = focusDetail
	}
	return func() tea.Msg {
		out, err := cl.LintPR(id)
		if err != nil {
			return errModalMsg{err}
		}
		return prLintMsg{id, out}
	}
}

// viewCursor is the actionable index of the view-selector item for a view.
func (m model) viewCursor(val string) int {
	for i, it := range m.prActionable() {
		if it.kind == "view" && it.value == val {
			return i
		}
	}
	return 0
}

// lintCommitNote names the commit a stored result describes, from the field that carries it rather
// than from the report prose the pane shows: a result about an older commit must be readable as one
// without reading it.
func lintCommitNote(sha string) string {
	if sha == "" {
		return ""
	}
	if len(sha) > 7 {
		sha = sha[:7]
	}
	return " · " + sha
}

// lintStatus summarizes a PR's stored gate result for the selector label — including the two states
// a queued gate spends real time in, which a label reading "done" would misreport as an answer.
func lintStatus(lint string) string {
	switch {
	case strings.TrimSpace(lint) == "":
		return "not gated"
	case strings.HasPrefix(lint, "gate PASS"):
		return "PASS"
	case strings.HasPrefix(lint, "gate FAIL"):
		return "FAIL"
	case strings.HasPrefix(lint, "gate queued"):
		return "queued"
	case strings.HasPrefix(lint, "gate running"):
		return "running"
	}
	return "done"
}

// selPRApproved reports whether the selected PR is approved (ready to merge).
func (m model) selPRApproved() bool {
	id := m.selID()
	for _, p := range m.state.PRs {
		if p.ID == id {
			return p.Status == "approved"
		}
	}
	return false
}

// openApproveMergeChoice offers approve (the human gate) then merge instead of failing an
// unapproved PR's merge; confirming emits approveMergeMsg so Update marks the row before the work.
func (m *model) openApproveMergeChoice(id string) {
	m.choice = choiceModalState{
		active: true, title: id + " isn't approved yet — approve and merge?",
		// Cancel first, as every other confirm here: the cursor opens on the first option, so
		// leading with the merge made Enter approve AND merge a PR nobody had reviewed.
		options: []string{"cancel", "approve & merge"}, values: []string{"cancel", "merge"},
		apply: func(v string) tea.Cmd {
			if v != "merge" {
				return nil
			}
			return func() tea.Msg { return approveMergeMsg{id: id} }
		},
	}
}

// markMerging shows a transient "merging" on the row (see prRows) until the hub confirms a status.
func (m *model) markMerging(id string) {
	if m.merging == nil {
		m.merging = map[string]bool{}
	}
	m.merging[id] = true
}

// mergeCmd merges an approved PR; both outcomes return mergeDoneMsg, which clears the marker.
func (m *model) mergeCmd(id string) tea.Cmd {
	cl := m.cl
	if cl == nil || id == "" {
		return nil
	}
	return func() tea.Msg {
		if _, err := cl.Merge(id); err != nil {
			return mergeDoneMsg{id: id, err: err}
		}
		st, _ := cl.State()
		return mergeDoneMsg{id: id, state: st}
	}
}

// approveMergeCmd approves then merges (the "approve & merge" path), reporting via mergeDoneMsg.
func (m *model) approveMergeCmd(id string) tea.Cmd {
	cl := m.cl
	if cl == nil || id == "" {
		return nil
	}
	return func() tea.Msg {
		if err := cl.ApprovePR(id); err != nil {
			return mergeDoneMsg{id: id, err: err}
		}
		if _, err := cl.Merge(id); err != nil {
			return mergeDoneMsg{id: id, err: err}
		}
		st, _ := cl.State()
		return mergeDoneMsg{id: id, state: st}
	}
}

// mergeDone drops the row's "merging" marker, then applies the fresh snapshot or shows the error.
func (m model) mergeDone(msg mergeDoneMsg) (tea.Model, tea.Cmd) {
	delete(m.merging, msg.id)
	if msg.err != nil {
		m.errText = msg.err.Error()
		return m, nil
	}
	m.state = msg.state
	m.reclamp()
	return m, tea.Batch(m.syncDetail(), m.agentLiveCmds())
}

// reconcileMerging clears a "merging" marker once a snapshot shows merged or the PR gone: a safety
// net for merges landing via SSE push rather than this client's own mergeDoneMsg.
func (m *model) reconcileMerging() {
	if len(m.merging) == 0 {
		return
	}
	status := make(map[string]string, len(m.state.PRs))
	for _, p := range m.state.PRs {
		status[p.ID] = p.Status
	}
	for id := range m.merging {
		if st, ok := status[id]; !ok || st == "merged" {
			delete(m.merging, id)
		}
	}
}

// openTaskModal shows a PR's linked task in the full-screen modal, identical to the Tasks tab's.
func (m *model) openTaskModal(t api.Task) {
	m.modalOverride = m.taskDetailFor(t, t.Description)
	m.modalOverrideTitle = "Task " + t.ID
	m.modal = true
	m.detail.SetHeight(modalContentHeight(m.h))
	m.detail.SetTotal(len(m.modalLines())) // wrapped count, matching the render
	m.detail.ScrollTop()
}

// openRejectForm opens a multiline textarea to reject a PR with a reason.
func (m *model) openRejectForm(prID string) {
	reason := newTextareaField("reason", "")
	cl := m.cl
	m.form.open("reject "+prID, []field{reason}, nil, func() tea.Cmd {
		text := reason.value()
		return func() tea.Msg {
			if cl == nil || strings.TrimSpace(text) == "" {
				return nil
			}
			if err := cl.RejectPR(prID, text); err != nil {
				return errModalMsg{err}
			}
			st, _ := cl.State()
			return polledMsg(st)
		}
	})
}

// openReviewForm opens a pre-filled, editable textarea to request an agentic review of a PR.
func (m *model) openReviewForm(prID string) {
	prompt := m.reviewPrompt // the editable default from the hub's review-prompt.txt
	if strings.TrimSpace(prompt) == "" {
		prompt = defaultReviewPrompt
	}
	req := newTextareaField("requirement", prompt)
	cl := m.cl
	m.form.open("agent-review of "+prID, []field{req}, nil, func() tea.Cmd {
		text := req.value()
		return func() tea.Msg {
			if cl == nil {
				return nil
			}
			if err := cl.RequestReview(prID, text); err != nil {
				return errModalMsg{err}
			}
			st, _ := cl.State()
			return polledMsg(st)
		}
	})
}

// prDetailW is the fixed width of the PRs tab's right detail column.
const prDetailW = 44

// prTable is the PRs list's columns, read by its header and every row alike — the shared set, with
// a status column narrowed to what a terminal pane can spare (-> table.PRList).
var prTable = table.PRList(9)

func (m model) prRows() []row {
	var foreign, local []row
	// Ordered by repo, the same call `sindri pr list` makes, so the two front-ends cannot drift onto
	// different orders. In repo scope that key gathers the foreign PRs waiting on the user together,
	// and the heading above them is what says so — a repo tag alone reads as a local row with an
	// unfamiliar tag, which is how the same rows were misread several times in one day (-> sectioned).
	for _, p := range api.SortedPRs(api.FilterPRs(m.prFilter, m.state.PRs), m.state.Projects) {
		switch { // still one predicate deciding what is listed: which group is all this asks
		case m.inScope(p.Project):
			local = append(local, m.prRow(p))
		case m.prVisible(p): // out of scope and listed anyway = waiting on the user elsewhere
			foreign = append(foreign, m.prRow(p))
		}
	}
	return m.listing(prTable, foreign, local)
}

// prRow renders one PR row: repo, id, status, age, who wrote it, who is reviewing it, its branch.
func (m model) prRow(p api.PR) row {
	status := api.StatusLabel(p.Status, p.Approvals)
	merging := m.merging[p.ID] && p.Status != "merged" // transient: the user triggered a merge, awaiting the hub
	if merging {
		status = "merging"
	}
	if p.Kind == "interim" { // the interim mark: a mid-task contribution, not a task-done PR
		status = theme.MarkPRInterim + status
	}
	// Cells styled independently, never nested, so a colour reset cannot bleed across the row —
	// the same shape the task and agent rows use.
	sc := prStatusStyle(p, m.state.Agents, merging)
	// Who is reviewing it, from the board — a dash where nobody is, so the column reads as
	// "waiting for a reviewer" rather than as missing.
	return row{prTable.Line(
		table.Cell{Text: m.repoName(p.Project), Style: m.repoStyle(p.Project).Render},
		table.Cell{Text: p.ID, Style: sc.Render},
		table.Cell{Text: status, Style: sc.Render},
		table.Cell{Text: theme.AttemptCell(p.Attempt), Style: sc.Render},
		table.Cell{Text: theme.Age(p.StatusChangedAt), Style: sc.Render},
		table.Cell{Text: theme.Age(p.CreatedAt), Style: sc.Render},
		table.Cell{Text: p.Agent, Style: sc.Render},
		table.Cell{Text: dash(p.Reviewer), Style: sc.Render},
		table.Cell{Text: p.Branch, Style: sc.Render},
	), p.ID}
}

// prTaskOpen finds the task a PR names, and whether it is still open — a task already closed or
// scrapped is not offered a second time.
func (m model) prTaskOpen(prID string) (taskID string, open bool) {
	for _, p := range m.state.PRs {
		if p.ID == prID {
			taskID = p.Task
			break
		}
	}
	if taskID == "" {
		return "", false
	}
	for _, t := range m.state.Tasks {
		if t.ID == taskID {
			return taskID, api.Open(t)
		}
	}
	return taskID, false
}

// openScrapPRChoice confirms scrapping a PR: branch gone, off the board, nobody asked to try again.
// The prompt names reject too, since the two are easy to confuse and only reject is recoverable.
// Its task is offered alongside, mirroring the task-scrap modal's own "+ PR" option — scrapping only
// the PR leaves the task open and claimable, so the same work is picked up and redone right away.
func (m *model) openScrapPRChoice(id string) {
	cl := m.cl
	taskID, taskOpen := m.prTaskOpen(id)
	opts, vals := []string{"cancel"}, []string{"cancel"}
	if taskOpen {
		opts = append(opts, "scrap PR only", "scrap PR + task "+taskID)
		vals = append(vals, "pr", "prtask")
	} else {
		opts = append(opts, "scrap "+id)
		vals = append(vals, "pr")
	}
	m.choice = choiceModalState{
		active: true, title: "scrap " + id + "? (deletes its branch; reject instead to send it back for another try)",
		options: opts, values: vals,
		apply: func(v string) tea.Cmd {
			switch v {
			case "pr":
				return mutateThenRefresh(cl, func() error { return cl.DiscardPR(id) })
			case "prtask":
				// withPRs=true: finishTask frees the task's holder and ScrapTask scraps this same
				// PR itself — the exact path the task-scrap modal's own "+ PR" option already takes.
				return mutateThenRefresh(cl, func() error { return cl.ScrapTask(taskID, false, true) })
			default:
				return nil
			}
		},
	}
}

// prKindLabel names a PR's kind for humans; "" reads as final, the historical default.
func prKindLabel(kind string) string {
	if kind == "interim" {
		return "interim (mid-task contribution)"
	}
	return "final (task done)"
}

// statusHeldFor suffixes the detail's status line with how long the PR has worn it (" for 3d"), or
// nothing at all on a row predating the column — an unadorned status beats one qualified by "-".
func statusHeldFor(p api.PR) string {
	if held := theme.Held(p.StatusChangedAt); held != theme.Unknown {
		return dimStyle.Render(" for " + held)
	}
	return ""
}

// prListHeight is the height of the top-left PR list; the big content pane gets the rest.
func (m model) prListHeight() int {
	n := len(m.rows())
	if n < 1 {
		n = 1
	}
	if cap := m.bodyHeight() * 2 / 5; n > cap {
		n = max(cap, 3)
	}
	return n
}

// prBody renders the PRs tab: PR list over the diff/lint pane, with the detail column right.
func (m model) prBody() string {
	h := m.bodyHeight()
	leftW := m.prContentWidth()

	listBox := pane(rowTexts(m.rows()), m.list, leftW, m.selRow())
	contentBox := pane(m.prContentLines(), m.detail, leftW, -1) // big pane: diff/lint
	leftCol := strings.Join([]string{listBox, hdivider(leftW), contentBox}, "\n")

	if !m.showDetail() { // § hid the right column — list + diff/lint take the full width
		return leftCol
	}
	rightW := m.w - leftW - 1
	lines, hl := m.prMetaLines(rightW)
	right := pane(lines, m.prMeta, rightW, hl)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, divider(h), right)
}

// prMetaLines is the right column's wrapped text and the line to highlight, or -1. Returned
// together because the highlight is an index INTO these lines: reclamp sizes the column and
// scrolls that line into view, and prBody draws it, so the two must be counting the same rows.
func (m model) prMetaLines(width int) ([]string, int) {
	items := wrapMeta(m.prMetaItems(), width)
	lines := make([]string, len(items))
	hl, ai := -1, 0 // highlight the focused actionable item when the right column has focus
	for i, it := range items {
		lines[i] = it.text
		if it.kind != "" {
			if m.focus == focusItems && ai == m.rightCursor {
				hl = i
			}
			ai++
		}
	}
	return lines, hl
}

// prContentWidth is the left pane's width, narrowed by the detail column; content wraps to it.
func (m model) prContentWidth() int {
	if m.showDetail() {
		return m.w - clampInt(prDetailW, 20, max(20, m.w-30)) - 1
	}
	return m.w
}

// prContentLines is the left pane for the selected view, word-wrapped rather than truncated.
func (m model) prContentLines() []string {
	return wrapContent(m.prRawContentLines(), m.prContentWidth())
}

// prRawContentLines is the unwrapped content: lint output if one was just run, otherwise the diff.
func (m model) prRawContentLines() []string {
	d := m.prDetail
	if d.PR.ID != m.selID() {
		return []string{dimStyle.Render("(loading…)")}
	}
	if m.prView == "lint" {
		if strings.TrimSpace(d.Lint) == "" {
			return []string{dimStyle.Render("(not gated — press L to ask)")}
		}
		return append([]string{dimStyle.Render("── lint ──"), ""},
			strings.Split(strings.TrimRight(d.Lint, "\n"), "\n")...)
	}
	if strings.TrimSpace(d.Diff) == "" {
		return []string{dimStyle.Render("(no diff)")}
	}
	return append([]string{dimStyle.Render("── diff ──"), ""}, renderDiff(d.Diff)...)
}

// metaItem is one right-column line; with kind set it can be focused, acted on (ENTER) or yanked.
type metaItem struct {
	text  string
	kind  string // "" plain · "agent" · "task" · "pr" · "path" · "view" · "url" · "mail" · "resume" · "mailbody"
	value string
}

// prMetaItems is the right detail column: metadata, actionable cross-references, then reviews.
func (m model) prMetaItems() []metaItem {
	d := m.prDetail
	if d.PR.ID != m.selID() {
		return []metaItem{{text: m.selID()}, {text: dimStyle.Render("(loading…)")}}
	}
	// View selectors drive the big content pane (ENTER switches it).
	view := func(val, label string) metaItem {
		mark := "  "
		if m.prView == val || (m.prView == "" && val == "diff") {
			mark = "▸ "
		}
		return metaItem{text: mark + label, kind: "view", value: val}
	}
	items := []metaItem{
		view("diff", "diff"),
		view("lint", "gate ("+lintStatus(d.Lint)+lintCommitNote(d.LintCommit)+")"),
		{text: ""},
	}
	items = append(items,
		metaItem{text: d.PR.ID},
		metaItem{text: "status: " + api.StatusLabel(d.PR.Status, api.ApprovalCount(d.Reviews)) + statusHeldFor(d.PR)},
		metaItem{text: "kind:   " + prKindLabel(d.PR.Kind)},
		metaItem{text: "agent:  " + d.PR.Agent, kind: "agent", value: d.PR.Agent},
	)
	if d.PR.Reviewer != "" { // only when somebody holds it: an empty line here would read as a gap
		items = append(items, metaItem{text: "review: " + d.PR.Reviewer, kind: "agent", value: d.PR.Reviewer})
	}
	// Absolute: `value` becomes a child process's working directory, so a relative one would break.
	if ws := m.agentWorkspacePath(d.PR.Agent); ws != "" {
		items = append(items, metaItem{text: "path:   " + ws, kind: "path", value: ws})
	}
	items = append(items,
		metaItem{text: "branch: " + d.PR.Branch + " → " + d.PR.Base},
		metaItem{text: ""}, metaItem{text: dimStyle.Render("── task ──")},
		metaItem{text: dash(d.Task.ID), kind: "task", value: d.Task.ID},
		metaItem{text: d.Task.Title},
	)
	// The lifecycle before anything long: what happened to this PR, in a few lines. The full
	// history stays at the bottom — it renders all 22 event types, which is what you want when
	// something went wrong and noise when you are asking "where is this up to".
	if life := api.PRLifecycle(d.History, d.Reviews); len(life) > 0 {
		items = append(items, metaItem{text: ""}, metaItem{text: dimStyle.Render("── lifecycle ──")})
		for _, ms := range life {
			items = append(items, metaItem{text: lifecycleLine(ms)})
		}
	}
	if d.PR.Feedback != "" {
		items = append(items, metaItem{text: ""}, metaItem{text: dimStyle.Render("── feedback ──")}, metaItem{text: d.PR.Feedback})
	}
	items = append(items, metaItem{text: ""}, metaItem{text: dimStyle.Render("── reviews ──")})
	if len(d.Reviews) == 0 {
		// From the constant, so a rebinding can't leave this hint pointing at the old key.
		items = append(items, metaItem{text: dimStyle.Render("(none — " + keyReview + " to request)")})
	}
	for _, r := range d.Reviews {
		items = append(items, metaItem{text: reviewLine(r)})
	}
	items = append(items, metaItem{text: ""}, metaItem{text: dimStyle.Render("── history ──")})
	// Newest-first so the latest event is at the top, like the agent activity log.
	for i := len(d.History) - 1; i >= 0; i-- {
		e := d.History[i]
		items = append(items, metaItem{text: fmt.Sprintf("%s  %-9s %s", dimStyle.Render(eventTime(e.TS)), e.Type, e.Payload)})
	}
	return items
}

// lifecycleLine renders one milestone: when, what, and who did it. The stamp is theme.Age's, as
// the task detail uses, so a time reads the same wherever it appears.
func lifecycleLine(ms api.PRMilestone) string {
	line := fmt.Sprintf("%s  %-12s", dimStyle.Render(eventTime(ms.At)), ms.Event)
	if ms.Who != "" {
		line += " by " + ms.Who
	}
	if ms.Note != "" {
		line += dimStyle.Render("  " + ms.Note)
	}
	return line
}

// wrapMeta wraps detail lines to the column width, so history, feedback and a long task/PR title
// read in full rather than losing their tail to the pane's ellipsis. Blank items, and any item
// already within width — the common case for the short actionable ones — pass through untouched,
// keeping their exact text so `y`/ENTER still act on what's shown. An item that overflows is
// word-wrapped; if it was actionable, only its first line keeps the kind/value, so the right-column
// cursor still lands on exactly one entry per source item.
func wrapMeta(items []metaItem, width int) []metaItem {
	if width <= 0 {
		return items
	}
	out := make([]metaItem, 0, len(items))
	for _, it := range items {
		if it.text == "" || ansi.StringWidth(it.text) <= width {
			out = append(out, it)
			continue
		}
		lines := strings.Split(ansi.Wrap(it.text, width, ""), "\n")
		out = append(out, metaItem{text: lines[0], kind: it.kind, value: it.value})
		for _, s := range lines[1:] {
			out = append(out, metaItem{text: s})
		}
	}
	return out
}

// prActionable is the focusable subset of the right column (j/k cycles these).
func (m model) prActionable() []metaItem {
	var out []metaItem
	for _, it := range m.prMetaItems() {
		if it.kind != "" {
			out = append(out, it)
		}
	}
	return out
}

// reviewLine summarizes a review item: its state, verdict, author and when, marking a planner's
// advisory badge for what it is — a second opinion, never the approval that satisfies the merge.
func reviewLine(r api.Review) string {
	switch {
	case r.Verdict != "":
		who := r.Author
		if r.Advisory {
			who += " (advisory)"
		}
		return fmt.Sprintf("• %s by %s at %s", r.Verdict, who, eventTime(r.VerdictAt))
	case r.Author != "":
		return fmt.Sprintf("• in review by %s", r.Author)
	default:
		return "• unassigned"
	}
}

// prIdentity says WHICH PR this is and where its work lives — the block you paste into a message
// or a ticket. Shared by the list yank and the full detail below so the two cannot drift: the
// workspace path was already in the interactive item column and in neither of these.
func (m model) prIdentity(d api.PRDetail) []string {
	ls := []string{
		fmt.Sprintf("%s   [%s]   by %s", d.PR.ID, api.StatusLabel(d.PR.Status, api.ApprovalCount(d.Reviews)), d.PR.Agent),
		fmt.Sprintf("task: %s  %s (%s)", d.Task.ID, d.Task.Title, d.Task.Status),
		fmt.Sprintf("branch %s → %s", d.PR.Branch, d.PR.Base),
	}
	// The field the yank was asked for, and the one nobody retypes. Absent once its author is
	// gone — a PR outlives the tree behind it.
	if ws := m.agentWorkspacePath(d.PR.Agent); ws != "" {
		ls = append(ls, "path: "+ws)
	}
	return ls
}

// prYankBlock is what `y` copies from the PRs list. Empty while the lazily-fetched detail is still
// another PR's, so the caller falls back to the id rather than pasting the wrong PR's fields.
func (m model) prYankBlock() []string {
	id := m.selID()
	if id == "" || m.prDetail.PR.ID != id {
		return nil
	}
	return m.prIdentity(m.prDetail)
}

// prDetailLines is the full PR detail for the ENTER modal: metadata, reviews, then the diff.
func (m model) prDetailLines() []string {
	id := m.selID()
	if id == "" {
		return []string{dimStyle.Render("(no PR)")}
	}
	d := m.prDetail
	if d.PR.ID != id {
		return []string{id, dimStyle.Render("(loading…)")}
	}
	ls := m.prIdentity(d)
	if life := api.PRLifecycle(d.History, d.Reviews); len(life) > 0 {
		ls = append(ls, "", "── lifecycle ──")
		for _, ms := range life {
			ls = append(ls, lifecycleLine(ms))
		}
	}
	if d.PR.Feedback != "" {
		ls = append(ls, "feedback: "+d.PR.Feedback)
	}
	for _, r := range d.Reviews {
		ls = append(ls, "", "── review ── "+reviewLine(r), "requirement: "+r.Requirement)
		if r.Result != "" {
			ls = append(ls, "findings: "+r.Result)
		}
	}
	if len(d.History) > 0 {
		ls = append(ls, "", "── history ──")
		for i := len(d.History) - 1; i >= 0; i-- {
			e := d.History[i]
			ls = append(ls, fmt.Sprintf("%s  %-9s %s", eventTime(e.TS), e.Type, e.Payload))
		}
	}
	ls = append(ls, "", "── diff ──")
	if strings.TrimSpace(d.Diff) == "" {
		ls = append(ls, dimStyle.Render("(no diff)"))
	} else {
		ls = append(ls, strings.Split(strings.TrimRight(d.Diff, "\n"), "\n")...)
	}
	return ls
}
