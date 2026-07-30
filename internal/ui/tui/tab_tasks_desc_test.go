package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TestTaskDescriptionShownWhileBrowsing: the board row already carries a task's
// description, so it must render in the detail pane as soon as the row is selected
// — without waiting for the lazy per-task detail fetch (m.taskDetail). Browsing
// used to show a blank description until that fetch landed.
func TestTaskDescriptionShownWhileBrowsing(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 0 // Tasks
	m.state = hub.BoardState{Tasks: []store.Task{
		{ID: "td-1", Title: "do the thing", Status: "open", Priority: "P1", Description: "BODY_FROM_BOARD"},
	}}
	m.cursor[0] = 0
	m.reclamp()
	// No detail fetch has happened yet (m.taskDetail is zero) — simulating browsing.

	lines := strings.Join(m.taskDetailLines(), "\n")
	if !strings.Contains(lines, "BODY_FROM_BOARD") {
		t.Fatalf("browsing should show the board row's description at once:\n%s", lines)
	}

	// A later detail fetch with a fresher body overrides the board row's copy.
	m.taskDetail = store.Task{ID: "td-1", Description: "FRESHER_BODY"}
	lines = strings.Join(m.taskDetailLines(), "\n")
	if !strings.Contains(lines, "FRESHER_BODY") || strings.Contains(lines, "BODY_FROM_BOARD") {
		t.Fatalf("detail read should refine the description:\n%s", lines)
	}
}
