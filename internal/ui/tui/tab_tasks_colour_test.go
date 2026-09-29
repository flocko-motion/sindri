package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// colourBoard is one task in each state the palette has to separate — and, crucially, EVERY
// combination of the two gates rather than only the ones that agree: a rejected task with no rating
// is where the row and the badge came apart, each looking plausible alone.
func colourBoard() []api.Task {
	return []api.Task{
		{ID: "td-unapproved", Status: "open", Approval: "pending", Priority: "P1"},
		{ID: "td-unrated", Status: "open"},
		{ID: "td-rejected", Status: "open", Approval: "rejected", Priority: "P1"},
		{ID: "td-rejected-unrated", Status: "open", Approval: "rejected"},
		{ID: "td-unapproved-unrated", Status: "open", Approval: "pending"},
		{ID: "td-working", Status: "in_progress", Priority: "P1"},
		{ID: "td-open", Status: "open", Priority: "P1"},
		{ID: "td-done", Status: "closed", Priority: "P1"},
	}
}

// gateOf is what taskRows passes as the row's approval state: the gate only where one still
// applies, which is what the rows themselves compute (a finished task's gate is spent).
func gateOf(t api.Task) string {
	if t.Approval != "" && !api.DoneStatus(t.Status) {
		return t.Approval
	}
	return ""
}

// TestRedIsExactlyWhatTheTasksBadgeCounts is the invariant, on the Tasks tab this time: the badge
// counts the rows stopped behind a gate the user opens, and red says this row is one of them. Both
// read api.TaskNeedsUser, so neither can drift while looking plausible alone.
func TestRedIsExactlyWhatTheTasksBadgeCounts(t *testing.T) {
	tasks := colourBoard()
	released := api.ReleasedByPriority(tasks)
	counted := 0
	for _, task := range tasks {
		style, _ := taskRowStyle(task, gateOf(task), released[task.ID])
		red := sameStyle(style, stCrit)
		needs := api.TaskNeedsUser(task, released[task.ID])
		if red != needs {
			t.Errorf("%s: red=%v but the badge counts it=%v — one of them is lying to the user",
				task.ID, red, needs)
		}
		if needs {
			counted++
		}
	}
	if got := api.CountTasksNeedingUser(tasks); got != counted {
		t.Errorf("the badge counts %d, the rows show %d red", got, counted)
	}
	if counted != 3 {
		t.Errorf("want the two unapproved tasks and the unrated one counted, got %d", counted)
	}
}

// TestBothGatesReadRed: approval and priority are the two gates that release work, so a task
// failing either is stopped — and neither is merely "your move", which is what yellow means.
func TestBothGatesReadRed(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.filter = 0, api.FilterAll
	m.state = api.BoardState{Tasks: colourBoard()}
	m.reclamp()

	rows := map[string]string{}
	for _, r := range m.taskRows() {
		rows[r.id] = r.text
	}
	if !strings.Contains(rows["td-unapproved"], theme.ApprovalLabel("pending")) {
		t.Errorf("the unapproved row should say so: %q", rows["td-unapproved"])
	}
	if !strings.Contains(rows["td-unrated"], "unrated") {
		t.Errorf("the unrated row should say so: %q", rows["td-unrated"])
	}
	// A rejected task has had its verdict: it is the author's move, so it must not read as
	// something the user has to act on — RATED OR NOT. Passing released=true here was the one value
	// that dodged the case the assertion is about.
	for _, released := range []bool{true, false} {
		if api.TaskNeedsUser(api.Task{ID: "x", Status: "open", Approval: "rejected"}, released) {
			t.Errorf("a rejected task (released=%v) waits on its author, not on the user", released)
		}
	}
	// And a done task is never counted, whatever gates it never passed.
	if api.TaskNeedsUser(api.Task{ID: "y", Status: "closed"}, false) {
		t.Error("a finished task is not waiting for anything")
	}
}

// TestTheDetailPairsCreatedWithChanged: the two timestamps read together, and the changed one is
// the field the active filter is built on — so an "n/a" here is the answer to "why did this task
// vanish from active the moment it closed?".
func TestTheDetailPairsCreatedWithChanged(t *testing.T) {
	m := newModel(nil, nil, "")
	m.tab, m.filter = 0, api.FilterAll
	m.state = api.BoardState{Tasks: []api.Task{
		{ID: "td-1", Title: "dated", Status: "open", Priority: "P1",
			CreatedAt: "2026-08-01T10:00:00Z", UpdatedAt: "2026-08-13T09:00:00Z"},
		{ID: "os-1", Title: "undated", Status: "closed", Priority: "P1", CreatedAt: "2026-08-01T10:00:00Z"},
	}}
	m.reclamp()

	detail := strings.Join(itemTexts(m.taskItemsFor(m.state.Tasks[0], "", nil)), "\n")
	if !strings.Contains(detail, "created:  "+theme.When("2026-08-01T10:00:00Z")) {
		t.Errorf("the created line should use the shared form:\n%s", detail)
	}
	if !strings.Contains(detail, "changed:  "+theme.When("2026-08-13T09:00:00Z")) {
		t.Errorf("the changed line should sit beside it, written the same way:\n%s", detail)
	}

	// A source with no timestamp: the blank IS the information, so it is shown rather than filled
	// in from the created time.
	detail = strings.Join(itemTexts(m.taskItemsFor(m.state.Tasks[1], "", nil)), "\n")
	if !strings.Contains(detail, "changed:  "+theme.Unknown) {
		t.Errorf("an undated task should say so plainly:\n%s", detail)
	}
	if strings.Contains(detail, "changed:  "+theme.When("2026-08-01T10:00:00Z")) {
		t.Error("the created time must not stand in for a missing changed time")
	}
}
