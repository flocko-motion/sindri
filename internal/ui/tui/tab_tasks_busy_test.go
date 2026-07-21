package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

func taskRowText(m model, id string) string {
	for _, r := range m.taskRows() {
		if r.id == id {
			return r.text
		}
	}
	return ""
}

// TestTaskBusyTransient: triggering a close/scrap shows the transient verb on the
// row at once (before the hub confirms), and it clears when a fresh board lands.
func TestTaskBusyTransient(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 0
	m.state = hub.BoardState{Tasks: []store.Task{{ID: "td-1", Title: "thing", Status: "open", Priority: "P1"}}}
	m.cursor[0] = 0
	m.reclamp()

	// Baseline: the row shows its real state, not a transient.
	if txt := taskRowText(m, "td-1"); !strings.Contains(txt, "open") || strings.Contains(txt, "closing") {
		t.Fatalf("baseline row = %q, want open and not closing", txt)
	}

	// Close: "closing" shows immediately, before any hub round-trip.
	m.markBusy("td-1", "closing")
	if txt := taskRowText(m, "td-1"); !strings.Contains(txt, "closing") {
		t.Fatalf("after trigger row = %q, want closing", txt)
	}

	// A fresh board showing it closed clears the transient (reconcile), and the row
	// shows the real state.
	m.state = hub.BoardState{Tasks: []store.Task{{ID: "td-1", Title: "thing", Status: "closed", Priority: "P1"}}}
	m.reconcileBusy()
	if m.busy["td-1"] != "" {
		t.Fatalf("marker should clear once the board confirms the task closed")
	}
	if txt := taskRowText(m, "td-1"); strings.Contains(txt, "closing") {
		t.Fatalf("after confirm row still shows closing: %q", txt)
	}
}

// TestReconcileBusyScrapped: a scrapped task vanishes from the board, so its
// transient marker is dropped too.
func TestReconcileBusyScrapped(t *testing.T) {
	m := newModel(nil, nil, "")
	m.markBusy("td-9", "deleting")
	m.state = hub.BoardState{Tasks: []store.Task{}} // td-9 gone
	m.reconcileBusy()
	if m.busy["td-9"] != "" {
		t.Fatalf("marker for a vanished (scrapped) task should be dropped")
	}
}

// TestTaskOpDoneClearsTransient: the done handler clears the marker on success
// (applying the fresh state) and on failure (surfacing the error modal).
func TestTaskOpDoneClearsTransient(t *testing.T) {
	// Success.
	m := newModel(nil, nil, "")
	m.markBusy("td-1", "closing")
	done := hub.BoardState{Tasks: []store.Task{{ID: "td-1", Status: "closed"}}}
	tm, _ := m.Update(taskOpDoneMsg{id: "td-1", state: done})
	got := tm.(model)
	if got.busy["td-1"] != "" {
		t.Fatalf("success should clear the busy marker")
	}
	if len(got.state.Tasks) != 1 || got.state.Tasks[0].Status != "closed" {
		t.Fatalf("success should apply the fresh state: %+v", got.state.Tasks)
	}

	// Failure: marker cleared, error surfaced.
	m2 := newModel(nil, nil, "")
	m2.markBusy("td-1", "deleting")
	tm2, _ := m2.Update(taskOpDoneMsg{id: "td-1", err: fmt.Errorf("boom")})
	got2 := tm2.(model)
	if got2.busy["td-1"] != "" {
		t.Fatalf("failure should clear the busy marker")
	}
	if got2.errText != "boom" {
		t.Fatalf("failure should surface the error modal, got %q", got2.errText)
	}
}
