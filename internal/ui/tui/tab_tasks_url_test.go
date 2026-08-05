package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestTaskURLMakesTheDetailReachable: ctrl+l only focuses the detail pane when it has at least one
// actionable item (onkey.go) — a GitHub issue with no parent/agent/PR set otherwise had NONE, so
// its detail pane could never be entered at all, same hotkey or not. The URL fixes that as a side
// effect of existing: it is the one thing there to focus.
func TestTaskURLMakesTheDetailReachable(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 0          // Tasks
	m.w, m.h = 120, 40 // wide enough for showDetail()
	m.state = api.BoardState{Tasks: []api.Task{
		{ID: "gh-12", Title: "an issue with no xrefs", Status: "open"},
	}}
	m.cursor[0] = 0
	m.reclamp()

	if got := len(m.actionableItems()); got != 0 {
		t.Fatalf("a task with no URL/parent/agent/PR should have 0 actionable items, got %d", got)
	}
	m.onKey("ctrl+l")
	if m.rightFocus {
		t.Fatal("ctrl+l must not focus an empty detail pane")
	}

	m.state.Tasks[0].URL = "https://github.com/acme/widgets/issues/12"
	if got := len(m.actionableItems()); got != 1 {
		t.Fatalf("the URL should be the one actionable item, got %d", got)
	}
	m.onKey("ctrl+l")
	if !m.rightFocus {
		t.Fatal("ctrl+l must focus the detail pane once it has the URL to land on")
	}
	it, ok := m.focusedItem()
	if !ok || it.kind != "url" || it.value != m.state.Tasks[0].URL {
		t.Fatalf("focused item = %+v, want the url xref", it)
	}
}

// TestTaskURLLineIsADashWhenAbsent: a plain task still shows a "url:" row for column-alignment
// with the other xref fields, but it is a placeholder, not something enter can act on.
func TestTaskURLLineIsADashWhenAbsent(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 0
	m.state = api.BoardState{Tasks: []api.Task{{ID: "td-1", Title: "plain", Status: "open"}}}
	m.cursor[0] = 0
	m.reclamp()

	lines := strings.Join(m.taskDetailLines(), "\n")
	if !strings.Contains(lines, "url:      -") {
		t.Fatalf("a task with no URL should show a dash placeholder:\n%s", lines)
	}
	for _, it := range m.taskItems() {
		if it.kind == "url" {
			t.Fatalf("an empty URL must not be actionable, got %+v", it)
		}
	}
}

// TestEnterCopiesTheTaskURL: enter on the focused URL item copies it and flashes a confirmation,
// instead of falling into the generic cross-reference path and opening a modal for a bare string.
func TestEnterCopiesTheTaskURL(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 0
	m.w, m.h = 120, 40
	url := "https://github.com/acme/widgets/issues/12"
	m.state = api.BoardState{Tasks: []api.Task{
		{ID: "gh-12", Title: "an issue", Status: "open", URL: url},
	}}
	m.cursor[0] = 0
	m.reclamp()
	m.onKey("ctrl+l")
	if !m.rightFocus {
		t.Fatal("precondition: the URL should have made the detail pane reachable")
	}

	m.onKey("enter")
	if m.modal {
		t.Fatal("enter on a url item must copy, not open the cross-reference modal")
	}
	if want := "copied URL: " + url; m.flash != want {
		t.Fatalf("flash = %q, want %q", m.flash, want)
	}
}
