package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// approvalBoard is an epic with a pending child, a pending grandchild, a rejected child and a
// closed one — so the count reflects only what a verdict would still decide.
func approvalBoard() api.BoardState {
	return api.BoardState{Tasks: []api.Task{
		{ID: "td-epic", Status: "open", Approval: "pending"},
		{ID: "td-a", Status: "open", ParentID: "td-epic", Approval: "pending"},
		{ID: "td-b", Status: "open", ParentID: "td-a", Approval: "pending"},
		{ID: "td-no", Status: "open", ParentID: "td-epic", Approval: "rejected"},
		{ID: "td-done", Status: "closed", ParentID: "td-epic", Approval: "pending"},
	}}
}

// TestPendingBelowCountsOnlyTheUndecided: the modal's number is a promise about what the wider
// approve changes, so a rejected child and a closed one must not inflate it.
func TestPendingBelowCountsOnlyTheUndecided(t *testing.T) {
	m := newModel(nil, nil, "")
	m.state = approvalBoard()
	for id, want := range map[string]int{"td-epic": 2, "td-a": 1, "td-b": 0, "td-no": 0} {
		if got := m.pendingBelow(id); got != want {
			t.Errorf("pendingBelow(%q) = %d, want %d", id, got, want)
		}
	}
}

// TestApproveChoiceOffersTheSubtasks: an epic's approve must offer the package-wide verdict, with
// the narrow one still there — approving a tree is the common case, never the only one.
func TestApproveChoiceOffersTheSubtasks(t *testing.T) {
	m := newModel(nil, nil, "")
	m.state = approvalBoard()

	m.openApproveChoice("td-epic", m.pendingBelow("td-epic"))
	want := []string{"cancel", "approve this task only", "approve task + 2 subtasks"}
	if got := m.choice.options; !slices.Equal(got, want) {
		t.Fatalf("approve modal options = %v, want %v", got, want)
	}
	if got := m.choice.values; !slices.Equal(got, []string{"cancel", "task", "tree"}) {
		t.Fatalf("approve modal values = %v", got)
	}
	if !strings.Contains(m.choice.title, "2 subtasks below await approval") {
		t.Errorf("the title should say why it is asking, got %q", m.choice.title)
	}
}

// TestApproveKeyAsksOnlyWhenThereIsAChoice drives the dispatcher: A on an epic must stop and ask,
// and A on a leaf must stay the single keypress it has always been.
func TestApproveKeyAsksOnlyWhenThereIsAChoice(t *testing.T) {
	for _, tc := range []struct {
		id       string
		wantAsks bool
	}{{"td-epic", true}, {"td-b", false}} {
		m := newModel(nil, nil, "")
		m.tab, m.scopeRepo, m.state = 0, false, approvalBoard()
		m.selectRow(tc.id)
		if got := m.selID(); got != tc.id {
			t.Fatalf("expected %s selected, got %q", tc.id, got)
		}
		m.onKey(keyApprove)
		if m.choice.active != tc.wantAsks {
			t.Errorf("%s: modal active = %v, want %v (title %q)", tc.id, m.choice.active, tc.wantAsks, m.choice.title)
		}
		if !tc.wantAsks && !strings.Contains(m.flash, "approving td-b") {
			t.Errorf("a leaf's approve should go straight through, flash = %q", m.flash)
		}
	}
}
