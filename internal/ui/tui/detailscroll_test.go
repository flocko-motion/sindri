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
// cursor can never reach them) must still be reachable — by scrolling, not by selecting.
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

	// PINNED BEHAVIOUR: J/K scroll the detail pane from EITHER pane — not focus-relative. Scrolling
	// without first focusing the right column (no ctrl+l) must reach the same content.
	var keys []string
	for i := 0; i < 15; i++ {
		keys = append(keys, "J")
	}
	afterUnfocused := screenshotWithDetail(b, task, 96, 24, keys...)
	if !strings.Contains(afterUnfocused, "finding number 19") {
		t.Errorf("J should scroll the detail pane into view even without ctrl+l first:\n%s", afterUnfocused)
	}

	afterFocused := screenshotWithDetail(b, task, 96, 24, append([]string{"ctrl+l"}, keys...)...)
	if !strings.Contains(afterFocused, "finding number 19") {
		t.Errorf("J should scroll the detail pane into view after ctrl+l too:\n%s", afterFocused)
	}
}
