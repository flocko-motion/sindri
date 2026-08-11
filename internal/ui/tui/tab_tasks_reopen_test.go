package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestOptionsReopensAClosedTaskOnTheTasksTab: `O` doubles as "an agent's options" (Agents) and
// "reopen" (Tasks) — the same key, gated by tab, the way `C` already covers close/clear-context.
func TestOptionsReopensAClosedTaskOnTheTasksTab(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.filter = 0, filterAll // the default filter hides closed tasks — this one needs to show
	m.state = api.BoardState{Tasks: []api.Task{{ID: "sd-abc123", Title: "a task", Status: "closed"}}}
	if id := m.selID(); id != "sd-abc123" {
		t.Fatalf("expected sd-abc123 selected, got %q", id)
	}
	m.onKey(keyOptions)
	if !m.form.active || !strings.Contains(m.form.title, "sd-abc123") {
		t.Errorf("%q should open sd-abc123's reopen form, got active=%v title=%q", keyOptions, m.form.active, m.form.title)
	}
}

// TestOptionsDoesNotReopenAnOpenTask: nothing to reopen on a task that isn't closed — `O` must not
// silently open the form and let a reason be filed against a task that never closed.
func TestOptionsDoesNotReopenAnOpenTask(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab = 0
	m.state = api.BoardState{Tasks: []api.Task{{ID: "sd-abc123", Title: "a task", Status: "open"}}}
	m.onKey(keyOptions)
	if m.form.active {
		t.Error("O should have nothing to do on a task that is not closed")
	}
}

// TestTaskReopenable is the gate itself: closed only, and false with nothing selected.
func TestTaskReopenable(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.filter = 0, filterAll
	m.state = api.BoardState{Tasks: []api.Task{
		{ID: "sd-1", Status: "closed"},
		{ID: "sd-2", Status: "open"},
	}}
	for _, id := range []string{"sd-1", "sd-2"} {
		m.selectRow(id)
		got := m.taskReopenable()
		want := id == "sd-1"
		if got != want {
			t.Errorf("taskReopenable(%s) = %v, want %v", id, got, want)
		}
	}
}

// TestReopenFormRequiresAReason: an empty reason must not close the form and read as a successful
// reopen — the same silent-empty shape the reject-PR form has, deliberately not copied here.
func TestReopenFormRequiresAReason(t *testing.T) {
	m := newModel(nil, nil, "")
	m.w, m.h = 100, 40
	m.openTaskReopenForm("sd-abc123")

	if cmd := m.form.update(keyMsg("ctrl+s")); cmd != nil {
		t.Fatal("an empty reason should not submit")
	}
	if !m.form.active {
		t.Error("the form must stay open on an empty reason")
	}
	if !strings.Contains(m.form.err, "say why") {
		t.Errorf("the form should say why it refused, got %q", m.form.err)
	}
}
