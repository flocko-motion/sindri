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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/flo-at/sindri/internal/api"
)

// defaultReviewPrompt pre-fills the Agentic Review instruction; the user edits it before dispatch.
const defaultReviewPrompt = "Review this PR for correctness, clarity, and fit to the task. Flag bugs, missing tests, and anything that should change."

// lintCmd runs the quality gate against the selected PR's worktree, into the big content pane.
func (m *model) lintCmd(id string) tea.Cmd {
	cl := m.cl
	m.flash = "linting " + id + "…"
	m.prDetail.Lint = "running lint…" // shown immediately; replaced by the result
	m.prView = "lint"
	m.rightFocus = true
	m.rightCursor = m.viewCursor("lint")
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

// lintStatus summarizes a PR's stored lint result for the selector label.
func lintStatus(lint string) string {
	switch {
	case strings.TrimSpace(lint) == "":
		return "not linted"
	case strings.HasPrefix(lint, "lint PASS"):
		return "PASS"
	case strings.HasPrefix(lint, "lint FAIL"):
		return "FAIL"
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

func (m model) prRows() []row {
	var out []row
	for _, p := range api.FilterPRs(m.prFilter, m.state.PRs) { // f-toggle: active by default
		if !m.inScope(p.Project) { // repo-scoped: only the active repo's PRs
			continue
		}
		repo := m.repoStyle(p.Project).Render(fmt.Sprintf("%-10.10s", m.repoName(p.Project)))
		status := api.StatusLabel(p.Status, p.Approvals)
		merging := m.merging[p.ID] && p.Status != "merged" // transient: the user triggered a merge, awaiting the hub
		if merging {
			status = "merging"
		}
		if p.Kind == "interim" { // ◇ = mid-task contribution (vs a final, task-done PR)
			status = "◇" + status
		}
		// Cells styled independently, never nested, so a colour reset cannot bleed across the row —
		// the same shape the task and agent rows use.
		sc := prStatusStyle(p, m.state.Agents, merging)
		// Who is reviewing it, from the board — a dash where nobody is, so the column reads as
		// "waiting for a reviewer" rather than as missing.
		out = append(out, row{strings.Join([]string{
			repo,
			sc.Render(fmt.Sprintf("%-14s", p.ID)),
			sc.Render(fmt.Sprintf("%-9s", status)),
			sc.Render(fmt.Sprintf("%4s", shortAge(p.CreatedAt))),
			sc.Render(fmt.Sprintf("%-10s", p.Agent)),
			sc.Render(fmt.Sprintf("%-10s", dash(p.Reviewer))),
			sc.Render(p.Branch),
		}, " "), p.ID})
	}
	return out
}

// openScrapPRChoice confirms scrapping a PR: branch gone, off the board, nobody asked to try again.
// The prompt names reject too, since the two are easy to confuse and only reject is recoverable.
func (m *model) openScrapPRChoice(id string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true, title: "scrap " + id + "? (deletes its branch; reject instead to send it back for another try)",
		options: []string{"cancel", "scrap " + id},
		values:  []string{"cancel", "scrap"},
		apply: func(v string) tea.Cmd {
			if v != "scrap" {
				return nil
			}
			return mutateThenRefresh(cl, func() error { return cl.DiscardPR(id) })
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

// shortAge renders an RFC3339 timestamp compactly ("3d", "now"); "-" when missing, not a fake age.
func shortAge(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
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

	listBox := pane(rowTexts(m.rows()), m.list, leftW, m.cursor[m.tab])
	contentBox := pane(m.prContentLines(), m.detail, leftW, -1) // big pane: diff/lint, J/K scrolls
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
			if m.rightFocus && ai == m.rightCursor {
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
			return []string{dimStyle.Render("(not linted — press L to run)")}
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
	kind  string // "" plain · "agent" · "task" · "pr" · "path" · "view" · "url"
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
		view("lint", "lint ("+lintStatus(d.Lint)+")"),
		{text: ""},
	}
	items = append(items,
		metaItem{text: d.PR.ID},
		metaItem{text: "status: " + api.StatusLabel(d.PR.Status, api.ApprovalCount(d.Reviews))},
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

// shellAt builds an interactive shell rooted at dir (for opening a workspace).
func shellAt(dir string) *exec.Cmd {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "bash"
	}
	c := exec.Command(sh)
	c.Dir = dir
	return marked(c)
}

// editorAtCmd opens the editor on an agent's live workspace; no materialization, it already exists.
func (m *model) editorAtCmd(dir string) tea.Cmd {
	m.flash = "opening " + dir + " in " + editorName() + "…"
	return func() tea.Msg { return editorReadyMsg(dir) }
}

// openEditorCmd materializes a PR (the same checkout `verify` shells into) and opens the editor.
func (m *model) openEditorCmd(id string) tea.Cmd {
	cl := m.cl
	m.flash = "opening " + id + " in " + editorName() + "…"
	return func() tea.Msg {
		path, err := cl.MaterializeReview(id)
		if err != nil {
			return errModalMsg{err}
		}
		return editorReadyMsg(path)
	}
}

// editorCandidates lists editors in the order a Unix tool looks: the user's choice, the
// distribution default, then vi, which POSIX requires, so the list can never come up empty.
func editorCandidates() []string {
	var out []string
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			out = append(out, v)
		}
	}
	return append(out, "sensible-editor", "editor", "vi")
}

// editorName is the editor that would run, for a flash message.
func editorName() string {
	for _, cand := range editorCandidates() {
		if bin, _, ok := resolveEditor(cand); ok {
			return filepath.Base(bin)
		}
	}
	return "an editor"
}

// resolveEditor splits a candidate ($EDITOR is often "code --wait") and checks PATH for the binary.
func resolveEditor(cand string) (bin string, args []string, ok bool) {
	fields := strings.Fields(cand)
	if len(fields) == 0 {
		return "", nil, false
	}
	p, err := exec.LookPath(fields[0])
	if err != nil {
		return "", nil, false
	}
	return p, fields[1:], true
}

// editorAt passes dir as the argument, so a file-browser editor (vim, emacs) lands on the tree
// rather than an empty buffer. nil when nothing is installed, for the caller to report.
func editorAt(dir string) *exec.Cmd {
	for _, cand := range editorCandidates() {
		bin, args, ok := resolveEditor(cand)
		if !ok {
			continue
		}
		c := exec.Command(bin, append(args, ".")...)
		c.Dir = dir
		return marked(c) // an editor with a built-in terminal is the same door as a shell
	}
	return nil
}

// verifyCmd materializes a PR for review, then signals the loop to open a shell there.
func (m *model) verifyCmd(id string) tea.Cmd {
	cl := m.cl
	m.flash = "materializing " + id + " for review…"
	return func() tea.Msg {
		path, err := cl.MaterializeReview(id)
		if err != nil {
			return errModalMsg{err}
		}
		return reviewReadyMsg(path)
	}
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
