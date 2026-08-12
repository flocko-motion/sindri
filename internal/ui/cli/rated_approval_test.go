package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

type fakeTasks struct {
	tasks []api.Task
	err   error
}

func (f fakeTasks) Tasks() ([]api.Task, error) { return f.tasks, f.err }

// TestRatingAPendingTaskSaysItIsNotReleased is the CLI half of the trap. Reporting only "set
// priority" on a task the approval gate still holds tells the user the work is ordered, when the
// claim queries will still pass it over — the same silence the TUI now breaks with a confirm.
func TestRatingAPendingTaskSaysItIsNotReleased(t *testing.T) {
	var out bytes.Buffer
	ratedApproval(&out, fakeTasks{tasks: []api.Task{
		{ID: "sd-1", Status: "open", Approval: "pending"},
	}}, "sd-1")
	got := out.String()
	for _, want := range []string{"awaits approval", "no worker can claim it", "task approve sd-1"} {
		if !strings.Contains(got, want) {
			t.Errorf("the advice is missing %q:\n%s", want, got)
		}
	}
}

// TestRatingAnApprovedTaskSaysNothing: the note explains a gate that is shut. Printing it for an
// open one would be noise on every rating, which is how advice stops being read.
func TestRatingAnApprovedTaskSaysNothing(t *testing.T) {
	for _, approval := range []string{"approved", ""} {
		var out bytes.Buffer
		ratedApproval(&out, fakeTasks{tasks: []api.Task{
			{ID: "sd-1", Status: "open", Approval: approval},
		}}, "sd-1")
		if out.Len() != 0 {
			t.Errorf("approval=%q: expected silence, got %q", approval, out.String())
		}
	}
}

// TestRatingReportsPendingChildrenWithoutApprovingThem: a scope can carry a rating to children the
// gate still holds. They are counted so the remaining verdicts are visible, and the flag that would
// take them is named rather than applied — each is its own decision.
func TestRatingReportsPendingChildrenWithoutApprovingThem(t *testing.T) {
	var out bytes.Buffer
	ratedApproval(&out, fakeTasks{tasks: []api.Task{
		{ID: "sd-1", Status: "open", Approval: "pending"},
		{ID: "sd-2", Status: "open", ParentID: "sd-1", Approval: "pending"},
		{ID: "sd-3", Status: "open", ParentID: "sd-1", Approval: "pending"},
	}}, "sd-1")
	got := out.String()
	if !strings.Contains(got, "2 tasks below") {
		t.Errorf("the pending children should be counted:\n%s", got)
	}
	if !strings.Contains(got, "--subtasks") {
		t.Errorf("the way to take them should be named, not taken:\n%s", got)
	}
}

// TestUnreadableBacklogSaysNothing: the advice is about the tree, so a backlog that did not read
// has nothing to say about it. Guessing would risk telling the user a rated task is stuck when it
// is not — and the rating itself succeeded either way.
func TestUnreadableBacklogSaysNothing(t *testing.T) {
	var out bytes.Buffer
	ratedApproval(&out, fakeTasks{err: errors.New("hub unreachable")}, "sd-1")
	if out.Len() != 0 {
		t.Errorf("expected silence when the backlog is unknown, got %q", out.String())
	}
}
