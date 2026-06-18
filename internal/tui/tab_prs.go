// package: tui / prs
// type:    ui (PRs tab)
// job:     the PRs tab content. Mirrors the Agents layout: a short PR list over
//          a big content pane (diff, or lint output after L) on the left, with
//          the fixed-width detail (metadata + linked task + reviews) on the
//          right. Detail is lazily fetched.
package tui

import (
	"fmt"
	"strings"

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
	m.prLint = "running lint…"
	return func() tea.Msg {
		out, err := cl.LintPR(id)
		if err != nil {
			return errModalMsg{err}
		}
		return prLintMsg{id, out}
	}
}

// openTaskModal shows a PR's linked task in the full-screen detail modal —
// identical to the Tasks-tab detail.
func (m *model) openTaskModal(t store.Task) {
	m.modalOverride = m.taskDetailFor(t, t.Description)
	m.modalOverrideTitle = "Task " + t.ID
	m.modal = true
	m.detail.SetHeight(modalContentHeight(m.h))
	m.detail.SetTotal(len(m.modalOverride))
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
	req := newTextareaField("requirement", defaultReviewPrompt)
	cl := m.cl
	m.form.open("agentic review of "+prID, []field{req}, nil, func() tea.Cmd {
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
		out = append(out, row{fmt.Sprintf("%-14s %-9s %-10s %s", p.ID, p.Status, p.Agent, p.Branch), p.ID})
	}
	return out
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
	rightW := clampInt(prDetailW, 20, max(20, m.w-30))
	leftW := m.w - rightW - 1

	listBox := pane(rowTexts(m.rows()), m.list, leftW, m.cursor[m.tab])
	contentBox := pane(m.prContentLines(), m.detail, leftW, -1) // big pane: diff/lint, J/K scrolls
	leftCol := strings.Join([]string{listBox, hdivider(leftW), contentBox}, "\n")

	var rv scroll.Viewport
	meta := m.prMetaLines()
	rv.Resize(h, len(meta))
	right := pane(meta, rv, rightW, -1)
	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, divider(h), right)
}

// prContentLines is the big left pane: the lint output if one was just run,
// otherwise the diff.
func (m model) prContentLines() []string {
	if strings.TrimSpace(m.prLint) != "" {
		return append([]string{dimStyle.Render("── lint ──"), ""},
			strings.Split(strings.TrimRight(m.prLint, "\n"), "\n")...)
	}
	d := m.prDetail
	if d.PR.ID != m.selID() {
		return []string{dimStyle.Render("(loading…)")}
	}
	if strings.TrimSpace(d.Diff) == "" {
		return []string{dimStyle.Render("(no diff)")}
	}
	return append([]string{dimStyle.Render("── diff ──"), ""},
		strings.Split(strings.TrimRight(d.Diff, "\n"), "\n")...)
}

// prMetaLines is the right detail column: PR metadata, the linked task, and the
// review items.
func (m model) prMetaLines() []string {
	d := m.prDetail
	if d.PR.ID != m.selID() {
		return []string{m.selID(), dimStyle.Render("(loading…)")}
	}
	ls := []string{
		d.PR.ID,
		"status: " + d.PR.Status,
		"agent:  " + d.PR.Agent,
		"branch: " + d.PR.Branch + " → " + d.PR.Base,
		"", dimStyle.Render("── task ──"),
		dash(d.Task.ID),
		d.Task.Title,
	}
	if d.PR.Feedback != "" {
		ls = append(ls, "", dimStyle.Render("── feedback ──"), d.PR.Feedback)
	}
	ls = append(ls, "", dimStyle.Render("── reviews ──"))
	if len(d.Reviews) == 0 {
		ls = append(ls, dimStyle.Render("(none — A to request)"))
	}
	for _, r := range d.Reviews {
		ls = append(ls, reviewLine(r))
	}
	return ls
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
	ls = append(ls, "", "── diff ──")
	if strings.TrimSpace(d.Diff) == "" {
		ls = append(ls, dimStyle.Render("(no diff)"))
	} else {
		ls = append(ls, strings.Split(strings.TrimRight(d.Diff, "\n"), "\n")...)
	}
	return ls
}
