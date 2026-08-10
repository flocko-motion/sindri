package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestUnratedReadsLikeUngated: no priority means no assignment, exactly as an open approval gate
// does, so the row says so in the same colour. A plain "open" claimed the task was available while
// no worker could ever be given it.
func TestUnratedReadsLikeUngated(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.filter = 0, filterAll
	m.state = api.BoardState{Tasks: []api.Task{
		{ID: "td-rated", Title: "rated", Status: "open", Priority: "P1"},
		{ID: "td-bare", Title: "unrated", Status: "open"},
	}}
	m.reclamp()

	if txt := taskRowText(m, "td-bare"); !strings.Contains(txt, "unrated") {
		t.Errorf("an unassignable task should say so: %q", txt)
	}
	if txt := taskRowText(m, "td-rated"); strings.Contains(txt, "unrated") {
		t.Errorf("a rated task is assignable and reads normally: %q", txt)
	}
}

// TestARatedEpicReleasesItsChildrenFromTheWarning is the case that makes a bare "has no priority"
// wrong: children of a rated epic are DELIBERATELY left unrated, and the epic's rating hands the
// whole tree out. Marking them would put a warning on nearly every subtask in the backlog.
func TestARatedEpicReleasesItsChildrenFromTheWarning(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.filter = 0, filterAll
	m.state = api.BoardState{Tasks: []api.Task{
		{ID: "td-epic", Title: "an epic", Status: "open", Priority: "P1"},
		{ID: "td-kid", Title: "a subtask", Status: "open", ParentID: "td-epic"},
		{ID: "td-grand", Title: "deeper", Status: "open", ParentID: "td-kid"},
		{ID: "td-orphan", Title: "loose", Status: "open"},
	}}
	m.reclamp()

	for _, id := range []string{"td-kid", "td-grand"} {
		if txt := taskRowText(m, id); strings.Contains(txt, "unrated") {
			t.Errorf("%s is released by its epic's rating: %q", id, txt)
		}
	}
	if txt := taskRowText(m, "td-orphan"); !strings.Contains(txt, "unrated") {
		t.Errorf("a loose unrated task has nothing releasing it: %q", txt)
	}
}

// TestTheGateStillWinsOverUnrated: a proposal is awaiting a verdict before it is awaiting a
// priority, and that is the verb the user reaches for — so the row keeps saying pending.
func TestTheGateStillWinsOverUnrated(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.filter = 0, filterAll
	m.state = api.BoardState{Tasks: []api.Task{
		{ID: "td-prop", Title: "proposed", Status: "open", Approval: "pending"},
	}}
	m.reclamp()

	txt := taskRowText(m, "td-prop")
	if !strings.Contains(txt, "pending") || strings.Contains(txt, "unrated") {
		t.Errorf("an ungated proposal reads as pending: %q", txt)
	}
}

// TestDoneTasksAreNeverMarkedUnrated: the warning is about work that cannot be handed out, and a
// finished task is not waiting for anything.
func TestDoneTasksAreNeverMarkedUnrated(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.filter = 0, filterAll
	m.state = api.BoardState{Tasks: []api.Task{
		{ID: "td-done", Title: "finished", Status: "closed"},
	}}
	m.reclamp()

	if txt := taskRowText(m, "td-done"); strings.Contains(txt, "unrated") {
		t.Errorf("a closed task needs no priority: %q", txt)
	}
}
