package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/api"
	"github.com/muesli/termenv"
)

// screenshotWithDetail is Screenshot, but with the selected task's lazily-fetched detail (its
// comment thread included) already landed — Screenshot alone never populates it, since that fetch
// normally comes from a live client this harness has none of.
func screenshotWithDetail(st api.BoardState, detail api.Task, w, h int, keys ...string) string {
	var tm tea.Model = newModel(nil, nil, "")
	m := tm.(model)
	m.w, m.h = w, h
	m.state = st
	m.taskDetail = detail
	m.detailKey = fmt.Sprintf("%d:%s", m.tab, detail.ID) // pretend the lazy fetch already landed
	m.reclamp()
	tm = m
	for _, k := range keys {
		tm, _ = tm.Update(keyMsg(k))
	}
	return tm.View()
}

// TestDetailScrollReachesContentPastTheLastActionableItem is the DONE WHEN this task names: a
// comment thread rendered after every cross-reference item (none of which have a `kind`, so the
// cursor can never reach them) must still be reachable — by scrolling, once the pane is focused.
// ctrl+l lands directly on that raw-content focus (focusDetail) here, since there is nothing
// actionable in it to step through instead (-> onKey's ctrl+l case; sd-57e895 replaced shift+J/K
// with this rather than leaving such content reachable only by an unconditional keypress).
func TestDetailScrollReachesContentPastTheLastActionableItem(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	var comments []api.Comment
	for i := 0; i < 20; i++ {
		comments = append(comments, api.Comment{Author: "eitri", Body: fmt.Sprintf("finding number %d", i), CreatedAt: "2026-01-01T00:00:00Z", Source: "sindri"})
	}
	task := api.Task{ID: "td-t1", Title: "a task with a long thread", Status: "open", Priority: "P1", Type: "task", Comments: comments}
	b := api.BoardState{Tasks: []api.Task{task}}

	before := screenshotWithDetail(b, task, 96, 24)
	if strings.Contains(before, "finding number 19") {
		t.Fatalf("the last comment should not already be in view with no scroll:\n%s", before)
	}

	// plain j scrolls one line at a time (unlike the old J, which stepped detailScrollStep at once),
	// so reaching the bottom of ~76 wrapped lines needs that many presses, not 15.
	keys := []string{"ctrl+l"}
	for i := 0; i < 90; i++ {
		keys = append(keys, "j")
	}
	after := screenshotWithDetail(b, task, 96, 24, keys...)
	if !strings.Contains(after, "finding number 19") {
		t.Errorf("j should scroll the detail pane into view once focused:\n%s", after)
	}
}

// TestCtrlLRevealsAnOffScreenCursor is the review round 5 blocker: scroll the detail pane deep with
// focusDetail, then step to focusItems — ctrl+l used to clamp rightCursor without ever scrolling to
// it, so the highlighted item ("agent: nori") could land outside the window the instant focusItems
// was entered, with j inheriting the same gap from the other side (only k's round-4 fix noticed it).
func TestCtrlLRevealsAnOffScreenCursor(t *testing.T) {
	var comments []api.Comment
	for i := 0; i < 25; i++ {
		comments = append(comments, api.Comment{Author: "eitri", Body: fmt.Sprintf("finding %d", i), CreatedAt: "2026-01-01T00:00:00Z", Source: "sindri"})
	}
	// Two actionable items (agent, url) so j has somewhere to move on to rather than immediately
	// falling into "past the last item, scroll instead" — a distinct, correct branch this test
	// is not about.
	task := api.Task{ID: "td-t1", Title: "a task", Status: "open", URL: "https://example.com/1", Comments: comments}
	m := newModel(nil, nil, "")
	m.tab = 0
	m.w, m.h = 96, 24
	m.state = api.BoardState{
		Tasks:  []api.Task{task},
		Agents: []api.AgentView{{Name: "nori", Status: "working", Task: "td-t1"}},
	}
	m.taskDetail = task
	m.detailKey = "0:td-t1"
	m.reclamp()

	m.onKey("ctrl+l") // -> focusDetail (this task has an actionable "agent" item, but that is the next stop)
	for i := 0; i < 20; i++ {
		m.onKey("j")
	}
	if m.detail.Offset == 0 {
		t.Fatal("precondition: j should have scrolled the detail pane deep")
	}
	m.onKey("ctrl+l") // -> focusItems, rightCursor clamped onto the first item ("agent: nori")
	if m.focus != focusItems {
		t.Fatalf("precondition: ctrl+l should reach focusItems, got %v", m.focus)
	}
	if line := m.focusedDetailLine(); line < m.detail.Offset || line >= m.detail.Offset+m.detail.Height {
		t.Errorf("entering focusItems left the highlighted item (line %d) outside the window [%d, %d)",
			line, m.detail.Offset, m.detail.Offset+m.detail.Height)
	}

	// And j, moving on from there, must not lose it either.
	m.onKey("j")
	if line := m.focusedDetailLine(); line < m.detail.Offset || line >= m.detail.Offset+m.detail.Height {
		t.Errorf("j left the highlighted item (line %d) outside the window [%d, %d)",
			line, m.detail.Offset, m.detail.Offset+m.detail.Height)
	}
}
