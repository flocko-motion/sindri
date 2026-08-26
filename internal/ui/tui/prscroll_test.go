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

// TestPRsDetailScrolls is the reported symptom, as a test: with the pane's raw content focused
// (focusDetail — sd-57e895's replacement for shift+J/K, which the footer used to advertise as
// "scroll detail"), plain j/k must move it. A keypress that leaves the pane identical is the bug —
// a user who presses it and sees nothing concludes the pane cannot scroll at all.
func TestPRsDetailScrolls(t *testing.T) {
	m := prScrollModel()
	if m.detail.Total <= m.detail.Height {
		t.Fatalf("precondition: the content must exceed the pane (total=%d height=%d)", m.detail.Total, m.detail.Height)
	}
	m.focus = focusDetail
	before := m.prBody()
	for i := 0; i < 3; i++ {
		m.onKey("j")
	}
	if m.detail.Offset == 0 {
		t.Errorf("j left the offset at 0 (height=%d total=%d)", m.detail.Height, m.detail.Total)
	}
	if after := m.prBody(); after == before {
		t.Error("j changed nothing on screen — the pane does not scroll")
	}
	// And back up again, to the top and no further.
	for i := 0; i < 20; i++ {
		m.onKey("k")
	}
	if m.detail.Offset != 0 {
		t.Errorf("k did not return to the top, offset=%d", m.detail.Offset)
	}
}

// TestHalfPageFollowsTheFocusedColumn: ctrl+d/ctrl+u are the coarse form of focusDetail's j/k, so on
// the tab with two scrollable regions they resolve their target the same way — whichever is
// focused. They used to half-page the diff whatever had focus, which is the same bug from the
// other end as the list moving while the detail was focused.
func TestHalfPageFollowsTheFocusedColumn(t *testing.T) {
	m := prMetaModel()
	m.focus = focusItems
	before := m.detail.Offset
	m.onKey("ctrl+d")
	paged := m.prMeta.Offset
	if paged == 0 {
		t.Fatalf("ctrl+d did not scroll the focused column (height=%d total=%d)", m.prMeta.Height, m.prMeta.Total)
	}
	if paged <= 1 {
		t.Errorf("ctrl+d moved %d lines, no more than a single j/k step — it should half-page", paged)
	}
	if m.detail.Offset != before {
		t.Error("half-paging the column must not also move the diff pane")
	}
	m.onKey("ctrl+u")
	if m.prMeta.Offset >= paged {
		t.Errorf("ctrl+u did not scroll back up: %d then %d", paged, m.prMeta.Offset)
	}
}

// TestPRsScrollFollowsTheFocusedPane pins the rule for this tab: focusDetail always scrolls the
// diff (m.detail) and focusItems always scrolls the metadata column (m.prMeta) — the two
// scrollable regions this tab has, each reached by its own stop on ctrl+l (-> scrollTarget). Every
// other tab is unaffected — there the focused pane and the detail pane are the same thing.
func TestPRsScrollFollowsTheFocusedPane(t *testing.T) {
	m := prScrollModel()
	m.focus = focusDetail
	m.onKey("j")
	if m.detail.Offset == 0 {
		t.Error("detail focused: j should scroll the diff pane")
	}

	m = prScrollModel()
	m.focus = focusItems
	m.onKey("j")
	if m.detail.Offset != 0 {
		t.Error("column focused: j must not scroll the diff pane")
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

// TestPRsMetaStepsItemsThenScrollsPastThem: focused on the metadata column, plain j/k first step
// rightCursor among its actionable cross-references (the view selector, the linked task, …); past
// the last one, further j falls through to scrolling the column line by line instead of clamping —
// reviews, findings and history are plain text with no cross-reference of their own, and with J/K
// gone this is the only line-granular way to reach them (review of sd-57e895's first submission:
// TestHalfPageFollowsTheFocusedColumn's ctrl+d/ctrl+u only cover the coarse case).
func TestPRsMetaStepsItemsThenScrollsPastThem(t *testing.T) {
	m := prMetaModel()
	m.focus = focusItems
	act := len(m.actionableItems())
	if act == 0 {
		t.Fatal("precondition: the column has actionable items to step through")
	}
	for i := 0; i < act-1; i++ { // walk onto the last item without overshooting yet
		m.onKey("j")
	}
	if m.rightCursor != act-1 {
		t.Fatalf("precondition: rightCursor = %d, want the last item %d", m.rightCursor, act-1)
	}
	if m.prMeta.Offset != 0 {
		t.Fatalf("precondition: stepping within the items must not scroll yet, offset=%d", m.prMeta.Offset)
	}
	for i := 0; i < 20; i++ { // past the last item: scroll deep, don't clamp rightCursor
		m.onKey("j")
	}
	if m.rightCursor != act-1 {
		t.Errorf("rightCursor moved to %d past the last item — it should stay put once scrolling takes over", m.rightCursor)
	}
	if m.prMeta.Offset != 20 {
		t.Fatalf("precondition: 20 j's past the last item should scroll 20 lines, got offset=%d", m.prMeta.Offset)
	}
}

// TestJThenKRoundTripsInFocusItems is the review round 6 invariant: from ANY position, j followed
// by k must return rightCursor and the column's offset to exactly what they were — j and k are
// each other's inverse everywhere, not just at the two boundaries. Round 5 broke this: k unwound
// past-the-end scroll only when it happened to be at the FIRST item, so one k deep in the trailing
// region (reviews/findings/history) discarded the whole scroll back to wherever rightCursor's own
// item sits — checked at three positions: mid-item, right at the boundary, and deep past it.
func TestJThenKRoundTripsInFocusItems(t *testing.T) {
	deep := func(m model, steps int) model {
		for i := 0; i < steps; i++ {
			m.onKey("j")
		}
		return m
	}
	act := len(prMetaModel().actionableItems())
	for _, tc := range []struct {
		name  string
		steps int
	}{
		{"mid-item", 1},
		{"at the last item", act - 1},
		{"deep in the trailing region", act - 1 + 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := prMetaModel()
			m.focus = focusItems
			m = deep(m, tc.steps)
			before := struct{ offset, cursor, excess int }{m.prMeta.Offset, m.rightCursor, m.detailExcess}

			m.onKey("j")
			m.onKey("k")

			if m.prMeta.Offset != before.offset || m.rightCursor != before.cursor || m.detailExcess != before.excess {
				t.Errorf("j then k did not round-trip: before=%+v after=(offset=%d cursor=%d excess=%d)",
					before, m.prMeta.Offset, m.rightCursor, m.detailExcess)
			}
		})
	}
}

// TestANoOpScrollNeverInflatesExcess is review round 8's blocker: ScrollDown/Up clamp at the pane's
// ends, so j past the point the pane can actually move is a true no-op — nothing to undo — but the
// old code counted the KEYPRESS, not its effect, so detailExcess grew on every press regardless.
// That phantom count is what let k walk backward through scroll that had never happened. Checked at
// both boundaries this can happen: a pane already scrolled to its max, and one that cannot scroll
// at all (Total <= Height) — the round-trip test above never reaches either, since prMetaModel has
// room to spare.
func TestANoOpScrollNeverInflatesExcess(t *testing.T) {
	t.Run("already at maxOffset", func(t *testing.T) {
		m := prMetaModel()
		m.focus = focusItems
		act := len(m.actionableItems())
		maxOffset := m.prMeta.Total - m.prMeta.Height
		for i := 0; i < act-1+maxOffset; i++ { // walk exactly onto the last scrollable line
			m.onKey("j")
		}
		if m.prMeta.Offset != maxOffset {
			t.Fatalf("precondition: expected to reach maxOffset %d, got %d", maxOffset, m.prMeta.Offset)
		}
		before := m.detailExcess

		m.onKey("j") // ScrollDown clamps here: nothing moves

		if m.prMeta.Offset != maxOffset {
			t.Errorf("offset moved past maxOffset: %d", m.prMeta.Offset)
		}
		if m.detailExcess != before {
			t.Errorf("a no-op j inflated detailExcess from %d to %d", before, m.detailExcess)
		}
		if m.flash == "" {
			t.Error("j with nowhere further to scroll should flash, not silently do nothing")
		}
	})

	t.Run("pane cannot scroll at all", func(t *testing.T) {
		m := prScrollModel() // its prMeta column is shorter than its own height — Total <= Height
		if m.prMeta.Total > m.prMeta.Height {
			t.Fatalf("precondition: this fixture's prMeta must not need scrolling (total=%d height=%d)", m.prMeta.Total, m.prMeta.Height)
		}
		m.focus = focusItems
		act := len(m.actionableItems())
		for i := 0; i < act-1; i++ { // onto the last item
			m.onKey("j")
		}
		for i := 0; i < 3; i++ { // three dead presses past it, exactly as measured in review
			m.onKey("j")
		}
		if m.detailExcess != 0 {
			t.Errorf("three no-op j's on an unscrollable pane produced excess=%d, want 0", m.detailExcess)
		}
		if m.rightCursor != act-1 {
			t.Errorf("rightCursor moved to %d on a dead j, want %d", m.rightCursor, act-1)
		}

		// With excess correctly at 0, a single k must move rightCursor at once — not burn phantom
		// unwind presses first.
		beforeCursor := m.rightCursor
		m.onKey("k")
		if m.rightCursor != beforeCursor-1 {
			t.Errorf("the first k after dead j's should step rightCursor immediately, got %d then %d", beforeCursor, m.rightCursor)
		}
	})
}

// TestKUnwindsScrollBeforeTheCursorEverywhere is the reviewer's exact reproduction: deep in the
// trailing region, a single k must move the pane by one line, never discard the scroll to jump
// back to rightCursor's own item.
func TestKUnwindsScrollBeforeTheCursorEverywhere(t *testing.T) {
	m := prMetaModel()
	m.focus = focusItems
	act := len(m.actionableItems())
	for i := 0; i < act-1+30; i++ {
		m.onKey("j")
	}
	scrolled := m.prMeta.Offset
	if scrolled == 0 {
		t.Fatal("precondition: j should have scrolled deep into the trailing region")
	}
	m.onKey("k")
	if m.prMeta.Offset != scrolled-1 {
		t.Errorf("k should move the pane by exactly one line, offset went from %d to %d", scrolled, m.prMeta.Offset)
	}
	if m.rightCursor != act-1 {
		t.Errorf("k unwinding scroll must not touch rightCursor, got %d", m.rightCursor)
	}

	// Once fully unwound, k resumes stepping the cursor — never "over-unwinding" into a jump.
	for m.detailExcess > 0 {
		m.onKey("k")
	}
	beforeCursor := m.rightCursor
	m.onKey("k")
	if m.rightCursor != beforeCursor-1 {
		t.Errorf("k after the scroll is fully unwound should step rightCursor by one, got %d then %d", beforeCursor, m.rightCursor)
	}
}

// TestHalfPageScrollUnwindsLikeJDoes is review round 9's blocker: ctrl+d/ctrl+u scroll the same
// pane j/k do, but did not fold into detailExcess — so the very next j/k, taking the cursor branch,
// called revealFocusedItem and snapped the half-page scroll straight back to wherever rightCursor's
// own item sits. Checked at rightCursor's two extremes: right after entering focusItems (cursor at
// the FIRST item, where j would otherwise jump to the second and discard the half-page) and at the
// LAST item (where k already had to unwind scroll from j, review round 6/8 — ctrl+d must feed that
// same mechanism, not bypass it).
func TestHalfPageScrollUnwindsLikeJDoes(t *testing.T) {
	t.Run("right after focusing, before any j", func(t *testing.T) {
		m := prMetaModel()
		m.focus = focusItems
		m.onKey("ctrl+d")
		scrolled := m.prMeta.Offset
		if scrolled == 0 {
			t.Fatal("precondition: ctrl+d should have scrolled the column")
		}
		beforeCursor := m.rightCursor

		m.onKey("j")

		if m.rightCursor != beforeCursor {
			t.Errorf("j right after a half-page scroll jumped the cursor to %d — it should keep scrolling instead", m.rightCursor)
		}
		if m.prMeta.Offset <= scrolled {
			t.Errorf("j should have scrolled one line further, offset went from %d to %d", scrolled, m.prMeta.Offset)
		}
	})

	t.Run("at the last item", func(t *testing.T) {
		m := prMetaModel()
		m.focus = focusItems
		act := len(m.actionableItems())
		for i := 0; i < act-1; i++ {
			m.onKey("j")
		}
		m.onKey("ctrl+d")
		scrolled := m.prMeta.Offset
		if scrolled == 0 {
			t.Fatal("precondition: ctrl+d should have scrolled the column")
		}

		m.onKey("k")

		if m.prMeta.Offset != scrolled-1 {
			t.Errorf("k should move the pane by exactly one line, offset went from %d to %d", scrolled, m.prMeta.Offset)
		}
		if m.rightCursor != act-1 {
			t.Errorf("k unwinding a half-page scroll must not touch rightCursor, got %d", m.rightCursor)
		}
	})
}

// TestPRsMetaScrollSurvivesAPoll: the column is offset-driven like the diff, so a board refresh
// must not drag it back. A scroll undone on the next tick is indistinguishable from one that never
// happened — which is how this whole class of complaint reads to a user.
func TestPRsMetaScrollSurvivesAPoll(t *testing.T) {
	m := prMetaModel()
	m.cl = &client.HTTP{}
	m.focus = focusItems
	m.syncDetail()
	m.onKey("ctrl+d")
	scrolled := m.prMeta.Offset
	if scrolled == 0 {
		t.Fatal("precondition: ctrl+d should have scrolled the column")
	}
	var tm tea.Model = m
	tm, _ = tm.Update(polledMsg(m.state))
	if got := tm.(model).prMeta.Offset; got != scrolled {
		t.Errorf("a poll moved the column from %d to %d", scrolled, got)
	}
}

// TestSingleRegionDetailScrollsOnceFocused: the same focusDetail stop that reaches the PRs diff
// reaches a single-region tab's own detail pane too (Tasks, Agents) — ctrl+l once, then plain j/k
// moves it. This replaces J/K's old "reaches it unconditionally" guarantee with "reaches it once
// you focus it," the rule the PRs tab always needed and every tab now shares.
func TestSingleRegionDetailScrollsOnceFocused(t *testing.T) {
	for _, tab := range []int{0, 1} { // Tasks and Agents: one scrollable region each
		m := newModel(nil, nil, "")
		m.tab, m.scopeRepo = tab, false
		m.w, m.h = 120, 24
		m.detail.Resize(5, 500) // a pane far shorter than its content
		m.focus = focusDetail
		m.onKey("j")
		if m.detail.Offset == 0 {
			t.Errorf("tab %d: j must scroll the detail pane once it is focused", tab)
		}
	}
}

// TestCtrlLReachesTheDiffWithTheColumnHidden is the review's second blocker: the diff (m.detail)
// renders full-width even with the metadata column hidden — prBody has its own branch for exactly
// that — so ctrl+l must still reach focusDetail there, the state a user most wants to read it in.
// The old guard (!m.showDetail()) blocked ctrl+l entirely, leaving the diff with no key at all.
func TestCtrlLReachesTheDiffWithTheColumnHidden(t *testing.T) {
	m := prScrollModel()
	m.hideDetail = true
	m.reclamp()
	m.onKey("ctrl+l")
	if m.focus != focusDetail {
		t.Fatalf("ctrl+l with the column hidden left focus at %v, want focusDetail", m.focus)
	}
	before := m.detail.Offset
	m.onKey("j")
	if m.detail.Offset == before {
		t.Error("j should scroll the diff once ctrl+l focused it")
	}
	// The column is not rendered, so its cross-references are not on screen either — a further
	// ctrl+l must cycle back to the list, not reach focusItems.
	m.onKey("ctrl+l")
	if m.focus != focusList {
		t.Errorf("ctrl+l again should cycle back to the list, not focusItems (got %v)", m.focus)
	}
}

// TestCtrlLReachesTheDiffOnANarrowTerminal is the same guarantee via the other route into
// !showDetail(): a terminal too narrow for the column, rather than § hiding it by hand.
func TestCtrlLReachesTheDiffOnANarrowTerminal(t *testing.T) {
	m := prScrollModel()
	m.w = detailMinWidth - 1
	m.reclamp()
	if m.showDetail() {
		t.Fatal("precondition: this width should hide the column")
	}
	m.onKey("ctrl+l")
	if m.focus != focusDetail {
		t.Fatalf("ctrl+l on a narrow terminal left focus at %v, want focusDetail", m.focus)
	}
}

// TestGAndCapitalGScrollTheFocusedDetailPane: with J/K gone, g/G are the only way to jump a
// focused, free-scrolling pane straight to its top or bottom rather than one line at a time.
func TestGAndCapitalGScrollTheFocusedDetailPane(t *testing.T) {
	m := prScrollModel()
	m.focus = focusDetail
	m.onKey("G")
	if m.detail.Offset == 0 {
		t.Fatal("G should scroll the diff to the bottom")
	}
	beforeCursor := m.cursor[m.tab]
	m.onKey("g")
	if m.detail.Offset != 0 {
		t.Error("g should scroll the diff back to the top")
	}
	if m.cursor[m.tab] != beforeCursor {
		t.Error("g/G in focusDetail must scroll the pane, not move the list cursor")
	}
	// An implemented-but-unadvertised binding is exactly what detail-pane-scroll-and-keymap-parity
	// exists to prevent (review round 3).
	if foot := m.contextFooter(); !strings.Contains(foot, "g/G") {
		t.Errorf("focusDetail's footer hint should advertise g/G, got %q", foot)
	}
}

// TestGInFocusItemsScrollsTheColumnNotTheList is the review round 6's second blocker: G had no
// focusItems case at all, so it fell through to moving the LIST selection — silently abandoning
// the PR the user was reading, its detail resynced and rightCursor reset out from under them. G
// now jumps the focused column to its bottom instead, past the last item, exactly like focusDetail.
func TestGInFocusItemsScrollsTheColumnNotTheList(t *testing.T) {
	m := prMetaModel()
	m.state.PRs = append(m.state.PRs, api.PR{ID: "pr-sd-2", Status: "open", Project: "repo", Branch: "sd-2", Base: "main"})
	m.reclamp()
	m.selectRow("pr-sd-1")
	m.focus = focusItems
	beforeSel := m.selID()

	m.onKey("G")
	if m.selID() != beforeSel {
		t.Errorf("G in focusItems changed the selected PR from %q to %q", beforeSel, m.selID())
	}
	if m.prMeta.Offset == 0 {
		t.Fatal("G in focusItems should scroll the metadata column to its bottom")
	}
	act := len(m.actionableItems())
	if m.rightCursor != act-1 {
		t.Errorf("G should land rightCursor on the last item %d, got %d", act-1, m.rightCursor)
	}
	if m.detailExcess == 0 {
		t.Error("G's jump past the last item should be recorded as excess, or k could not unwind it")
	}

	// k, one line at a time, must be able to walk all the way back from G's jump.
	for m.detailExcess > 0 {
		m.onKey("k")
	}
	if m.prMeta.Offset != 0 {
		t.Errorf("unwinding G's excess one k at a time should reach offset 0, got %d", m.prMeta.Offset)
	}
}

// TestLReachesTheReportWithTheColumnHidden is the review's third blocker: L (run the gate) set
// focus = focusItems unconditionally in two places — lintCmd, on the keystroke, and again in the
// prLintMsg handler once the report lands — even with the metadata column hidden, leaving the
// report with no scroll key at all (j/k stepped over cross-references that were not on screen, and
// ctrl+d/ctrl+u resolved to that same column).
func TestLReachesTheReportWithTheColumnHidden(t *testing.T) {
	m := prScrollModel()
	m.hideDetail = true
	m.cl = &client.HTTP{}
	m.reclamp()

	m.onKey(" ")     // L commits, so it lives behind the space prefix
	m.onKey(keyLint) // lintCmd's own synchronous state, before the async fetch lands
	if m.focus != focusDetail {
		t.Fatalf("focus right after L = %v, want focusDetail with the column hidden", m.focus)
	}

	var lint strings.Builder
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&lint, "finding %03d\n", i)
	}
	var tm tea.Model = m
	tm, _ = tm.Update(prLintMsg{pr: "pr-sd-1", text: lint.String()})
	m = tm.(model)
	if m.focus != focusDetail {
		t.Fatalf("focus once the report landed = %v, want focusDetail with the column hidden", m.focus)
	}
	before := m.detail.Offset
	m.onKey("j")
	if m.detail.Offset == before {
		t.Error("j should scroll the landed lint report")
	}
}

// TestLReachesTheReportOnANarrowTerminal is the same guarantee via the other route into
// !showDetail(): a terminal too narrow for the column, rather than § hiding it by hand.
func TestLReachesTheReportOnANarrowTerminal(t *testing.T) {
	m := prScrollModel()
	m.w = detailMinWidth - 1
	m.cl = &client.HTTP{}
	m.reclamp()
	if m.showDetail() {
		t.Fatal("precondition: this width should hide the column")
	}

	m.onKey(" ") // L commits, so it lives behind the space prefix
	m.onKey(keyLint)
	if m.focus != focusDetail {
		t.Fatalf("focus right after L = %v, want focusDetail on a narrow terminal", m.focus)
	}
}
