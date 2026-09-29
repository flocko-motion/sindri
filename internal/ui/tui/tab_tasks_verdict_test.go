package tui

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// taskTabWith puts one task on the Tasks tab, selected — the row the menu is asked about.
func taskTabWith(t api.Task) model {
	m := newModel(nil, nil, "/r/one")
	m.tab, m.scopeRepo = 0, false
	m.state = api.BoardState{Tasks: []api.Task{t}}
	m.w, m.h = 120, 24
	m.reclamp()
	return m
}

// TestARejectedTaskStillOffersAVerdict closes the half of tasks.md 6.4 that was a plain gap rather
// than a question. onKey gates A/R on taskGated — pending OR rejected, the states the approval gate
// still holds — while the keymap row gated them on pending alone, and the menu refuses a letter it
// never offered. So the rejected branch of onKey could not be reached from the keyboard at all, and
// a rejected proposal had no route back through the TUI.
func TestARejectedTaskStillOffersAVerdict(t *testing.T) {
	m := taskTabWith(api.Task{ID: "sd-1", Title: "a proposal", Status: "open", Approval: "rejected"})
	if !menuHas(m, "A approve") {
		t.Errorf("a rejected but open task must still offer approve:\n%s", menuText(m))
	}
	if !menuHas(m, "R reject") {
		t.Errorf("a rejected but open task must still offer reject:\n%s", menuText(m))
	}
}

// TestAPendingTaskStillOffersAVerdict: the widening must not cost the case that already worked.
func TestAPendingTaskStillOffersAVerdict(t *testing.T) {
	m := taskTabWith(api.Task{ID: "sd-1", Title: "a proposal", Status: "open", Approval: "pending"})
	if !menuHas(m, "A approve") {
		t.Errorf("a task awaiting a verdict must offer approve:\n%s", menuText(m))
	}
}

// TestASettledTaskOffersNoVerdict is the other boundary: a verdict on work that has already ended
// decides nothing, so the menu answers "what can I do with THIS" by leaving it out.
func TestASettledTaskOffersNoVerdict(t *testing.T) {
	m := taskTabWith(api.Task{ID: "sd-1", Title: "done", Status: "closed", Approval: "rejected"})
	if menuHas(m, "A approve") {
		t.Errorf("a closed task must not offer a verdict:\n%s", menuText(m))
	}
	m = taskTabWith(api.Task{ID: "sd-2", Title: "ruled on", Status: "open", Approval: "approved"})
	if menuHas(m, "A approve") {
		t.Errorf("an approved task is past the gate and must not offer a verdict:\n%s", menuText(m))
	}
}
