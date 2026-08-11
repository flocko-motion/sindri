package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
)

// prScrollModel is the PRs tab showing a diff far longer than the pane, which is the only state in
// which scrolling means anything.
func prScrollModel() model {
	m := newModel(nil, nil, "")
	m.tab, m.scopeRepo = 2, false
	m.w, m.h = 120, 40
	pr := api.PR{ID: "pr-sd-1", Status: "open", Project: "repo", Agent: "bombur", Branch: "sd-1", Base: "main"}
	m.state = api.BoardState{
		Projects: []api.Project{{Tag: "repo", Path: "/r/one"}},
		PRs:      []api.PR{pr},
	}
	var diff strings.Builder
	diff.WriteString("diff --git a/x b/x\n")
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&diff, "+unique-line-%03d\n", i)
	}
	m.prDetail = api.PRDetail{
		PR:   pr,
		Task: api.Task{ID: "sd-1", Title: "a task", Status: "in_progress"},
		Diff: diff.String(),
	}
	m.prView = "diff"
	m.reclamp()
	return m
}

// TestPRsDetailScrolls is the reported symptom, as a test. The footer advertises J/K as "scroll
// detail", so a keypress that leaves the pane identical is the bug — a user who presses it and
// sees nothing concludes the pane cannot scroll at all.
func TestPRsDetailScrolls(t *testing.T) {
	m := prScrollModel()
	if m.detail.Total <= m.detail.Height {
		t.Fatalf("precondition: the content must exceed the pane (total=%d height=%d)", m.detail.Total, m.detail.Height)
	}
	before := m.prBody()
	for i := 0; i < 3; i++ {
		m.onKey("J")
	}
	if m.detail.Offset == 0 {
		t.Errorf("J left the offset at 0 (height=%d total=%d)", m.detail.Height, m.detail.Total)
	}
	if after := m.prBody(); after == before {
		t.Error("J changed nothing on screen — the pane does not scroll")
	}
	// And back up again, to the top and no further.
	for i := 0; i < 20; i++ {
		m.onKey("K")
	}
	if m.detail.Offset != 0 {
		t.Errorf("K did not return to the top, offset=%d", m.detail.Offset)
	}
}

// TestPRsDetailHalfPages: ctrl+d/ctrl+u drive the same viewport, so they must move the same
// content — by more than J does, since that is the distinction the two pairs of keys carry.
func TestPRsDetailHalfPages(t *testing.T) {
	m := prScrollModel()
	m.onKey("ctrl+d")
	paged := m.detail.Offset
	if paged == 0 {
		t.Fatalf("ctrl+d did not scroll (height=%d total=%d)", m.detail.Height, m.detail.Total)
	}
	if paged <= detailScrollStep {
		t.Errorf("ctrl+d moved %d lines, no more than J's %d — it should half-page", paged, detailScrollStep)
	}
	m.onKey("ctrl+u")
	if m.detail.Offset >= paged {
		t.Errorf("ctrl+u did not scroll back up: %d then %d", paged, m.detail.Offset)
	}
}

// TestPRsScrollFollowsTheFocusedPane pins the rule for this tab, which supersedes sd-ac8831's
// "J/K are not focus-relative". That decision was made for tabs with ONE detail pane; the PRs tab
// has two scrollable regions and one pair of keys, so the pair follows the focus. Every other tab
// is unaffected — there the focused pane and the detail pane are the same thing.
func TestPRsScrollFollowsTheFocusedPane(t *testing.T) {
	m := prScrollModel()
	m.rightFocus = false
	m.onKey("J")
	if m.detail.Offset == 0 {
		t.Error("list focused: J should scroll the diff pane")
	}

	m = prScrollModel()
	m.rightFocus = true
	m.onKey("J")
	if m.detail.Offset != 0 {
		t.Error("column focused: J must not scroll the diff pane")
	}
}

// prMetaModel is a PR whose right column overflows: a rejection reason, several reviews with
// findings, and a long history — the material that actually sits below the fold in practice.
func prMetaModel() model {
	m := prScrollModel()
	m.prDetail.PR.Feedback = strings.Repeat("a long rejection reason. ", 20)
	for i := 0; i < 12; i++ {
		m.prDetail.Reviews = append(m.prDetail.Reviews, api.Review{
			Author: fmt.Sprintf("rev%02d", i), Verdict: "approved",
			Requirement: "check it", Result: fmt.Sprintf("finding-%02d", i),
		})
		m.prDetail.History = append(m.prDetail.History, api.Event{
			TS: "2026-08-11T10:00:00Z", Type: "event", Payload: fmt.Sprintf("history-%02d", i),
		})
	}
	m.reclamp()
	return m
}

// TestPRsMetaColumnCanScroll is the defect behind the report. The right column built a viewport
// locally on every render, so its Offset was always zero and nothing could move it — J/K drive the
// diff pane on this tab, and there was no second viewport to drive. Everything past the first
// screenful of reviews and history was unreachable.
func TestPRsMetaColumnCanScroll(t *testing.T) {
	m := prMetaModel()
	lines, _ := m.prMetaLines(max(1, m.w-m.prContentWidth()-1))
	if len(lines) <= m.prMeta.Height {
		t.Fatalf("precondition: the column must overflow (lines=%d height=%d)", len(lines), m.prMeta.Height)
	}
	if m.prMeta.Total != len(lines) {
		t.Errorf("the column's viewport does not know its own length: total=%d want %d", m.prMeta.Total, len(lines))
	}
}

// TestPRsMetaScrollsWhenFocused is the fix for the report. The right column holds the reviews,
// their findings, the rejection feedback and the whole history — none of it actionable, so no
// cursor ever visits it, and J/K drive the diff on this tab. It therefore had no key at all, and
// everything past the first screenful was unreachable. Focused, J/K now move it.
func TestPRsMetaScrollsWhenFocused(t *testing.T) {
	m := prMetaModel()
	m.rightFocus = true
	before := m.detail.Offset
	for i := 0; i < 3; i++ {
		m.onKey("J")
	}
	if m.prMeta.Offset == 0 {
		t.Errorf("J did not scroll the focused column (height=%d total=%d)", m.prMeta.Height, m.prMeta.Total)
	}
	if m.detail.Offset != before {
		t.Error("scrolling the column must not also move the diff pane")
	}
	for i := 0; i < 20; i++ {
		m.onKey("K")
	}
	if m.prMeta.Offset != 0 {
		t.Errorf("K did not return the column to the top, offset=%d", m.prMeta.Offset)
	}
}

// TestPRsDiffStillScrollsFromTheList: the focus rule must not cost the behaviour that already
// worked. With the list focused — the ordinary state — J/K still drive the diff.
func TestPRsDiffStillScrollsFromTheList(t *testing.T) {
	m := prMetaModel()
	m.rightFocus = false
	for i := 0; i < 3; i++ {
		m.onKey("J")
	}
	if m.detail.Offset == 0 {
		t.Error("J from the list must still scroll the diff pane")
	}
	if m.prMeta.Offset != 0 {
		t.Error("scrolling the diff must not also move the right column")
	}
}

// TestPRsMetaScrollSurvivesAPoll: the column is offset-driven like the diff, so a board refresh
// must not drag it back. A scroll undone on the next tick is indistinguishable from one that never
// happened — which is how this whole class of complaint reads to a user.
func TestPRsMetaScrollSurvivesAPoll(t *testing.T) {
	m := prMetaModel()
	m.cl = &client.HTTP{}
	m.rightFocus = true
	m.syncDetail()
	for i := 0; i < 3; i++ {
		m.onKey("J")
	}
	scrolled := m.prMeta.Offset
	if scrolled == 0 {
		t.Fatal("precondition: J should have scrolled the column")
	}
	var tm tea.Model = m
	tm, _ = tm.Update(polledMsg(m.state))
	if got := tm.(model).prMeta.Offset; got != scrolled {
		t.Errorf("a poll moved the column from %d to %d", scrolled, got)
	}
}
