package tui

import (
	"fmt"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// longListModel is the Tasks tab with more rows than fit and a detail pane taller than its pane —
// so both the list and the detail have somewhere to go, and a key that moves the wrong one shows.
func longListModel() model {
	m := newModel(nil, nil, "")
	m.tab, m.scopeRepo = 0, false
	m.w, m.h = 120, 40
	tasks := make([]api.Task, 0, 80)
	for i := 0; i < 80; i++ {
		tasks = append(tasks, api.Task{
			ID: fmt.Sprintf("sd-%03d", i), Title: fmt.Sprintf("task %03d", i),
			Type: "task", Status: "open", Approval: "approved",
		})
	}
	m.state = api.BoardState{Projects: []api.Project{{Tag: "repo", Path: "/r/one"}}, Tasks: tasks}
	m.reclamp()
	m.detail.Resize(30, 500) // a detail far longer than its pane, and deep enough to half-page
	return m
}

// TestHalfPageMovesTheListWhenTheListIsFocused: the list has no separate viewport to scroll — the
// cursor carries the view, and half a body height is the half-page (-> view-tui, the selected line
// stays in view).
func TestHalfPageMovesTheListWhenTheListIsFocused(t *testing.T) {
	m := longListModel()
	m.rightFocus = false
	m.onKey("ctrl+d")
	if m.cursor[0] == 0 {
		t.Fatal("ctrl+d with the list focused must move the list cursor")
	}
	if m.detail.Offset != 0 {
		t.Errorf("it must not scroll the detail pane as well, offset=%d", m.detail.Offset)
	}
	moved := m.cursor[0]
	m.onKey("ctrl+u")
	if m.cursor[0] >= moved {
		t.Errorf("ctrl+u did not move back up: %d then %d", moved, m.cursor[0])
	}
}

// TestHalfPageMovesTheDetailWhenTheDetailIsFocused is the reported bug: focus the detail pane,
// press ctrl+d, and the LIST moved — the keys decided per tab instead of asking which pane was in
// play, so they contradicted J/K, which had asked all along.
func TestHalfPageMovesTheDetailWhenTheDetailIsFocused(t *testing.T) {
	m := longListModel()
	m.rightFocus = true
	before := m.cursor[0]
	m.onKey("ctrl+d")
	if m.detail.Offset == 0 {
		t.Fatal("ctrl+d with the detail focused must scroll the detail pane")
	}
	if m.cursor[0] != before {
		t.Errorf("the list cursor moved under a detail scroll: %d then %d", before, m.cursor[0])
	}
	paged := m.detail.Offset
	if paged <= detailScrollStep {
		t.Errorf("ctrl+d moved %d lines, no more than J's %d — it should half-page", paged, detailScrollStep)
	}
	m.onKey("ctrl+u")
	if m.detail.Offset >= paged {
		t.Errorf("ctrl+u did not scroll back up: %d then %d", paged, m.detail.Offset)
	}
}
