// package: tui / prs
// type:    ui (PRs tab)
// job:     the PRs tab content. Mirrors the Agents layout: a short PR list over
//          a big content pane (diff, or lint output after L) on the left, with
//          the fixed-width detail (metadata + linked task + reviews) on the
//          right. Detail is lazily fetched.
// limits:  renders PR state and actions; verdict/merge logic is the hub's
//          (-> client / workflow_pr.go).
package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/tui/scroll"
)

// defaultReviewPrompt pre-fills the Agentic Review instruction; the user edits
// it before dispatching.
const defaultReviewPrompt = "Review this PR for correctness, clarity, and fit to the task. Flag bugs, missing tests, and anything that should change."

// lintCmd runs the quality gate against the selected PR's worktree, showing the
// result in the big content pane.
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

// openApproveMergeChoice handles pressing merge on a not-yet-approved PR: rather
// than failing the merge, offer to approve (the human gate) and merge in one step.
func (m *model) openApproveMergeChoice(id string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true, title: id + " isn't approved yet — approve and merge?",
		options: []string{"approve & merge", "cancel"}, values: []string{"merge", "cancel"},
		apply: func(v string) tea.Cmd {
			if v != "merge" || cl == nil {
				return nil
			}
			return func() tea.Msg {
				if err := cl.ApprovePR(id); err != nil {
					return errModalMsg{err}
				}
				if _, err := cl.Merge(id); err != nil {
					return errModalMsg{err}
				}
				st, _ := cl.State()
				return polledMsg(st)
			}
		},
	}
}

// openTaskModal shows a PR's linked task in the full-screen detail modal —
// identical to the Tasks-tab detail.
func (m *model) openTaskModal(t store.Task) {
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

// openReviewForm opens a textarea (pre-filled, editable) to request an agentic
// review of a PR.
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
	for _, p := range m.state.PRs {
		repo := projectStyle(p.Project).Render(fmt.Sprintf("%-10.10s", m.repoName(p.Project)))
		out = append(out, row{fmt.Sprintf("%s %-14s %-9s %4s %-10s %s", repo, p.ID, p.Status, shortAge(p.CreatedAt), p.Agent, p.Branch), p.ID})
	}
	return out
}

// shortAge renders how long ago an RFC3339 timestamp was, compactly ("3d", "2h",
// "5m", "now"); "-" when it's empty or unparseable, so a missing timestamp reads as
// unknown rather than a bogus age.
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

// prListHeight is the height of the short PR-list region (top-left); the big
// content pane gets the rest.
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

// prBody renders the PRs tab: a short PR list over the big content pane (diff /
// lint) on the left, with the metadata + task + reviews detail on the right.
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
	items := wrapMeta(m.prMetaItems(), rightW)
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
	var rv scroll.Viewport
	rv.Resize(h, len(lines))
	right := pane(lines, rv, rightW, hl)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, divider(h), right)
}

// prContentWidth is the display width of the big content pane — the left column,
// narrowed by the detail column when it's shown. The diff/lint text is wrapped to
// this so nothing overflows off the right edge.
func (m model) prContentWidth() int {
	if m.showDetail() {
		return m.w - clampInt(prDetailW, 20, max(20, m.w-30)) - 1
	}
	return m.w
}

// prContentLines is the big left pane, driven by the selected view (diff/lint),
// word-wrapped to the pane width so the full PR is readable (no truncation).
func (m model) prContentLines() []string {
	return wrapContent(m.prRawContentLines(), m.prContentWidth())
}

// prRawContentLines builds the unwrapped content: lint output if one was just
// run, otherwise the diff.
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

// metaItem is one line of the right detail column. An actionable item (kind
// set) can be focused (h/l, then j/k) and acted on (ENTER) or yanked (y).
type metaItem struct {
	text  string
	kind  string // "" plain · "agent" · "task" · "path"
	value string
}

// prMetaItems is the right detail column: PR metadata (with the agent, its
// workspace, and the linked task as actionable cross-references), then reviews.
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
		metaItem{text: "status: " + d.PR.Status},
		metaItem{text: "agent:  " + d.PR.Agent, kind: "agent", value: d.PR.Agent},
	)
	if ws := m.agentWorkspace(d.PR.Agent); ws != "" {
		items = append(items, metaItem{text: "path:   " + ws, kind: "path", value: ws})
	}
	items = append(items,
		metaItem{text: "branch: " + d.PR.Branch + " → " + d.PR.Base},
		metaItem{text: ""}, metaItem{text: dimStyle.Render("── task ──")},
		metaItem{text: dash(d.Task.ID), kind: "task", value: d.Task.ID},
		metaItem{text: d.Task.Title},
	)
	if d.PR.Feedback != "" {
		items = append(items, metaItem{text: ""}, metaItem{text: dimStyle.Render("── feedback ──")}, metaItem{text: d.PR.Feedback})
	}
	items = append(items, metaItem{text: ""}, metaItem{text: dimStyle.Render("── reviews ──")})
	if len(d.Reviews) == 0 {
		items = append(items, metaItem{text: dimStyle.Render("(none — A to request)")})
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

// wrapMeta word-wraps each plain detail line to the column width, so long text
// (history payloads, feedback) is readable in full rather than truncated with an
// ellipsis. Actionable items (and blank spacers) are passed through untouched so
// the right-column focus cursor still maps 1:1 to its actionable rows.
func wrapMeta(items []metaItem, width int) []metaItem {
	if width <= 0 {
		return items
	}
	wrap := lipgloss.NewStyle().Width(width)
	out := make([]metaItem, 0, len(items))
	for _, it := range items {
		if it.kind != "" || it.text == "" {
			out = append(out, it)
			continue
		}
		for _, s := range strings.Split(wrap.Render(it.text), "\n") {
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

// agentWorkspace returns an agent's (repo-relative) worktree path, or "".
func (m model) agentWorkspace(name string) string {
	for _, a := range m.state.Agents {
		if a.Name == name {
			return a.Workspace
		}
	}
	return ""
}

// shellAt builds an interactive shell rooted at dir (for opening a workspace).
func shellAt(dir string) *exec.Cmd {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "bash"
	}
	c := exec.Command(sh)
	c.Dir = dir
	return c
}

// verifyCmd materializes a PR into the review workspace, then signals the loop
// to open a shell there.
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

// reviewLine summarizes a review item: its state, verdict, and author.
func reviewLine(r store.Review) string {
	switch {
	case r.Verdict != "":
		return fmt.Sprintf("• %s by %s", r.Verdict, r.Author)
	case r.Author != "":
		return fmt.Sprintf("• in review by %s", r.Author)
	default:
		return "• unassigned"
	}
}

// prDetailLines is the full PR detail (for the ENTER modal): metadata, reviews
// (with their requirement + findings), then the diff.
func (m model) prDetailLines() []string {
	id := m.selID()
	if id == "" {
		return []string{dimStyle.Render("(no PR)")}
	}
	d := m.prDetail
	if d.PR.ID != id {
		return []string{id, dimStyle.Render("(loading…)")}
	}
	ls := []string{
		fmt.Sprintf("%s   [%s]   by %s", d.PR.ID, d.PR.Status, d.PR.Agent),
		fmt.Sprintf("task: %s  %s (%s)", d.Task.ID, d.Task.Title, d.Task.Status),
		fmt.Sprintf("branch %s → %s", d.PR.Branch, d.PR.Base),
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
