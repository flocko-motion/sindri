package store

import "testing"

// TestAPackageWithAPendingSubtaskIsNotOffered is the assignment half of sd-d38859. A package is
// handed out when its PLAN is settled, and a subtask still awaiting a verdict is invisible to
// OpenSubtasks — so the package looked workable, the worker took it, built the approved siblings,
// and was told the feature was finished over a subtask nobody had ruled on.
func TestAPackageWithAPendingSubtaskIsNotOffered(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "F", Status: "open", Priority: "P1"},
		{ID: "A", Status: "open", Priority: "P1", ParentID: "F"},
		{ID: "B", Status: "open", Priority: "P1", ParentID: "F"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustContainers(t, p)); !eq(got, []string{"F"}) {
		t.Fatalf("precondition: an ungated package should be offered, got %v", got)
	}

	// The ordinary case: a planner edits B, which returns it to awaiting-review (-> CmdEditTask).
	if err := p.SetApproval("B", "pending", ""); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustContainers(t, p)); len(got) != 0 {
		t.Errorf("a package with a subtask awaiting a verdict must not be offered, got %v", got)
	}
	// And the verdict releases it, or the guard would be a permanent block rather than a wait.
	if err := p.SetApproval("B", "approved", ""); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustContainers(t, p)); !eq(got, []string{"F"}) {
		t.Errorf("the verdict must release the package, got %v", got)
	}
}

// TestAPendingDescendantBlocksAtAnyDepth: the direct-child guard this replaces missed a deeper
// edit entirely, and OpenSubtasks reaches any depth — a fix that stopped at one level would leave
// the same hole one rung down.
func TestAPendingDescendantBlocksAtAnyDepth(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "F", Status: "open", Priority: "P1"},
		{ID: "MID", Status: "open", Priority: "P1", ParentID: "F"},
		{ID: "DEEP", Status: "open", Priority: "P1", ParentID: "MID"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetApproval("DEEP", "pending", ""); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustContainers(t, p)); len(got) != 0 {
		t.Errorf("a grandchild awaiting a verdict must block its package too, got %v", got)
	}
}

// TestARejectedDescendantBlocksNothing: nothing ever clears a rejection, so blocking on one parks
// the package for ever — the same line the completion half already draws.
func TestARejectedDescendantBlocksNothing(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "F", Status: "open", Priority: "P1"},
		{ID: "A", Status: "open", Priority: "P1", ParentID: "F"},
		{ID: "B", Status: "open", Priority: "P1", ParentID: "F"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetApproval("B", "rejected", "not this way"); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustContainers(t, p)); !eq(got, []string{"F"}) {
		t.Errorf("a ruled-on subtask must not hold its package, got %v", got)
	}
}

// TestAClosedPendingSubtaskBlocksNothing: the question is whether there is unsettled WORK beneath
// the package, and a closed row carries no work whatever its stale verdict says.
func TestAClosedPendingSubtaskBlocksNothing(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "F", Status: "open", Priority: "P1"},
		{ID: "A", Status: "open", Priority: "P1", ParentID: "F"},
		{ID: "B", Status: "closed", Priority: "P1", ParentID: "F"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetApproval("B", "pending", ""); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustContainers(t, p)); !eq(got, []string{"F"}) {
		t.Errorf("a closed subtask must not hold its package, got %v", got)
	}
}
