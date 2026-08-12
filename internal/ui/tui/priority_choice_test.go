package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/api"
)

// priorityBoard is an epic with a rated child, an unrated grandchild, an unrated child and a closed
// one; parentStatus decides whether the package is still being worked as one.
func priorityBoard(parentStatus string) api.BoardState {
	return api.BoardState{Tasks: []api.Task{
		{ID: "td-epic", Status: parentStatus, Priority: "P1"},
		{ID: "td-a", Status: "open", ParentID: "td-epic", Priority: "P0"},
		{ID: "td-deep", Status: "open", ParentID: "td-a"},
		{ID: "td-b", Status: "open", ParentID: "td-epic"},
		{ID: "td-done", Status: "closed", ParentID: "td-epic"},
	}}
}

// TestPriorityAsksForScopeOnlyOverATree: a leaf has nothing below it, so the scope step would be a
// keypress with one answer. Over a tree it is asked, with the three scopes and their real counts.
func TestPriorityAsksForScopeOnlyOverATree(t *testing.T) {
	m := newModel(nil, nil, "")
	m.state = priorityBoard("open")

	m.openPriorityChoice("td-b")
	if cmd := m.choice.apply("P2"); cmd != nil {
		if _, ok := cmd().(openPriorityScopeMsg); ok {
			t.Error("a leaf's rating must not ask how far it carries")
		}
	}

	m.openPriorityChoice("td-epic")
	cmd := m.choice.apply("P2")
	if cmd == nil {
		t.Fatal("picking a priority over a tree produced no command")
	}
	msg, ok := cmd().(openPriorityScopeMsg)
	if !ok {
		t.Fatalf("want the scope step, got %T", cmd())
	}
	m.openPriorityScopeChoice(msg.id, msg.code)
	if got := m.choice.values; !slices.Equal(got, []string{"task", "unrated", "all"}) {
		t.Errorf("scope values = %v", got)
	}
	want := []string{"this task only", "+ the 2 tasks below with no priority set", "+ all 3 tasks below (overwrite)"}
	if got := m.choice.options; !slices.Equal(got, want) {
		t.Errorf("scope options = %v, want %v", got, want)
	}
}

// TestTheScopeModalNeverClaimsToReleaseWork is the honesty the semantics demand: under an OPEN parent
// the children are worked as one package, so carrying a rating to them sets the order they come in and
// releases nothing. A modal that said otherwise would report work as available that no worker can take.
func TestTheScopeModalNeverClaimsToReleaseWork(t *testing.T) {
	m := newModel(nil, nil, "")
	m.state = priorityBoard("open")
	m.openPriorityScopeChoice("td-epic", "P2")
	note := m.choice.note
	if !strings.Contains(note, "ORDER") || !strings.Contains(note, "not whether they're released") {
		t.Errorf("an open package's scope modal must say a rating only orders its subtasks, got %q", note)
	}
	if strings.Contains(note, "claimable") {
		t.Errorf("it must not offer releasability it cannot deliver, got %q", note)
	}

	// Closed parent: each child stands alone, and now a rating IS what makes it claimable.
	m.state = priorityBoard("closed")
	m.openPriorityScopeChoice("td-epic", "P2")
	if !strings.Contains(m.choice.note, "claimable") {
		t.Errorf("with the parent ended, the modal must say the rating makes each child claimable, got %q", m.choice.note)
	}

	// The sentence has to be on screen, not merely in the state — a note the renderer drops is a
	// modal that quietly stopped explaining itself.
	if screen := choiceModal(m.choice, 100, 24); !strings.Contains(screen, "claimable") {
		t.Errorf("the rendered modal does not show its note:\n%s", screen)
	}
}

// TestApproveHandsOnToThePriority is the flow the split cost us: approving FEELS like releasing work,
// and an unrated task stays claimable by nobody. So the approve's follow-up is the priority picker —
// and only when nothing rates the task yet, since asking about a rated one is friction in reverse.
func TestApproveHandsOnToThePriority(t *testing.T) {
	m := newModel(nil, nil, "")
	m.state = api.BoardState{Tasks: []api.Task{
		{ID: "td-bare", Status: "open", Approval: "pending"},
		{ID: "td-rated", Status: "open", Approval: "pending", Priority: "P2"},
		{ID: "td-epic", Status: "open", Approval: "pending", Priority: "P1"},
		{ID: "td-kid", Status: "open", ParentID: "td-epic", Approval: "pending"},
	}}
	then := m.priorityAfterApprove("td-bare")
	if then == nil {
		t.Fatal("an unrated task's approve must hand on to the priority")
	}
	if got, ok := then().(openPriorityChoiceMsg); !ok || string(got) != "td-bare" {
		t.Errorf("the follow-up should open the picker for td-bare, got %#v", then())
	}
	if m.priorityAfterApprove("td-rated") != nil {
		t.Error("a task that already carries a priority needs nothing more")
	}
	// A subtask inside a rated epic is released by that epic, so it is not owed a rating either.
	if m.priorityAfterApprove("td-kid") != nil {
		t.Error("a child under a rated epic is already released — its own rating only orders it")
	}
}

// TestAFailedApproveRatesNothing: the follow-up is what an approve earned, so an approve that failed
// must not open a picker whose write would land on a task still gated.
func TestAFailedApproveRatesNothing(t *testing.T) {
	failed := afterTaskOp(
		func() tea.Msg { return taskOpDoneMsg{id: "td-1", err: errors.New("still pending")} },
		func() tea.Msg { return openPriorityChoiceMsg("td-1") },
	)
	done, ok := failed().(taskOpDoneMsg)
	if !ok {
		t.Fatalf("want the op's own result, got %T", failed())
	}
	if done.then != nil {
		t.Error("a failed approve must not chain the priority picker")
	}
}
