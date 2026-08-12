package tui

import (
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/api"
)

// TestFilterCyclesThroughFour: "f" used to wrap open->closed->all->open (mod 3); active is a
// fourth stop, so the cycle must wrap at 4 without skipping or repeating a state. It starts from
// "active", which is where the tab opens.
func TestFilterCyclesThroughFour(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 0
	if m.filter != filterActive {
		t.Fatalf("the Tasks tab opens on %q, want active", filterNames[m.filter])
	}
	want := []int{filterOpen, filterClosed, filterAll, filterActive}
	for _, w := range want {
		m.onKey(keyFilter)
		if m.filter != w {
			t.Fatalf("after cycling, filter = %d, want %d", m.filter, w)
		}
	}
}

// TestActiveFilterIncludesOpenAndRecentlyChanged: the filter this task adds — open tasks, plus
// anything (open or not) whose status changed inside the window, so a task closed moments ago
// doesn't disappear from view the instant it's done.
func TestActiveFilterIncludesOpenAndRecentlyChanged(t *testing.T) {
	now := time.Now().UTC()
	m := newModel(nil, nil, "")
	m.tab = 0
	m.filter = filterActive
	m.state = api.BoardState{Tasks: []api.Task{
		{ID: "td-1", Title: "open, no timestamp", Status: "open"},
		{ID: "td-2", Title: "closed just now", Status: "closed", UpdatedAt: now.Format(time.RFC3339)},
		{ID: "td-3", Title: "closed long ago", Status: "closed", UpdatedAt: now.Add(-3 * time.Hour).Format(time.RFC3339)},
		{ID: "td-4", Title: "closed, no timestamp", Status: "closed"},
	}}

	seen := map[string]bool{}
	for _, r := range m.taskRows() {
		seen[r.id] = true
	}
	if !seen["td-1"] {
		t.Error("an open task must show under active, even with no timestamp")
	}
	if !seen["td-2"] {
		t.Error("a task closed inside the window must show under active")
	}
	if seen["td-3"] {
		t.Error("a task closed outside the window must not show under active")
	}
	if seen["td-4"] {
		t.Error("a closed task with no timestamp has no evidence of recency and must not show under active")
	}
}
