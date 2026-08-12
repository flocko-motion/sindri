package store

import "testing"

// TestAnEpicsPriorityReleasesItsWholeTree: priority is the readiness signal, and a package is
// claimed as one unit — so rating the epic alone releases it, and its children come along unrated.
// Rating each child would be busywork that changes nothing about what gets handed out.
func TestAnEpicsPriorityReleasesItsWholeTree(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "td-123", Status: "open", Priority: "P1"},
		{ID: "td-a", Status: "open", ParentID: "td-123"},
		{ID: "td-b", Status: "open", ParentID: "td-123"},
		{ID: "td-c", Status: "open", ParentID: "td-123"},
	}); err != nil {
		t.Fatal(err)
	}

	containers, err := p.OpenContainers()
	if err != nil {
		t.Fatal(err)
	}
	if len(containers) != 1 || containers[0].ID != "td-123" {
		t.Fatalf("OpenContainers: want [td-123], got %v", ids(containers))
	}
	children, err := p.OpenSubtasks("td-123")
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(children); len(got) != 3 {
		t.Fatalf("OpenSubtasks: want all three unrated children, got %v", got)
	}
	// And no child is offered on its own: that would split the tree across agents.
	if got := mustLeaves(t, p); len(got) != 0 {
		t.Fatalf("OpenLeaves: want none, got %v", got)
	}
}

// TestAFeaturesWorkIsItsLEAVES: a direct child can itself be an epic, and it is not work — its own
// children are. Serving the middle node got it closed on the next checkpoint over four open
// children, and the grandchildren were never offered at all: too deep for the feature loop, and
// barred from the leaf loop by an open parent. Unrated is not what excluded them; depth was.
func TestAFeaturesWorkIsItsLeaves(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "td-feat", Status: "open", Priority: "P1"},
		{ID: "td-flat", Status: "open", ParentID: "td-feat"},
		{ID: "td-mid", Status: "open", ParentID: "td-feat"}, // an epic in the middle
		{ID: "td-deep1", Status: "open", ParentID: "td-mid"},
		{ID: "td-deep2", Status: "open", ParentID: "td-mid"},
	}); err != nil {
		t.Fatal(err)
	}
	got := ids(mustSubtasks(t, p, "td-feat"))
	want := map[string]bool{"td-flat": true, "td-deep1": true, "td-deep2": true}
	if len(got) != len(want) {
		t.Fatalf("OpenSubtasks: want the three leaves at any depth, got %v", got)
	}
	for _, id := range got {
		if !want[id] {
			t.Errorf("OpenSubtasks served %q; td-mid is a parent, so it is not work", id)
		}
	}
	// Once its own children are done, the middle epic is finished — never a leaf to hand out.
	for _, id := range []string{"td-deep1", "td-deep2"} {
		if err := p.UpsertTask(Task{ID: id, Status: "closed", ParentID: "td-mid"}); err != nil {
			t.Fatal(err)
		}
	}
	if open, err := p.OpenChildIDs("td-mid"); err != nil || len(open) != 0 {
		t.Fatalf("OpenChildIDs after both closed: got %v (%v)", open, err)
	}
}

// TestAnUnratedEpicStaysInTheBacklog is the other half: an epic with rated children is still not
// released, because the epic is what an agent claims.
func TestAnUnratedEpicStaysInTheBacklog(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "td-123", Status: "open"},
		{ID: "td-a", Status: "open", Priority: "P0", ParentID: "td-123"},
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := p.OpenContainers(); err != nil || len(got) != 0 {
		t.Fatalf("OpenContainers: want none (epic unrated), got %v (%v)", ids(got), err)
	}
	if got := mustLeaves(t, p); len(got) != 0 {
		t.Fatalf("OpenLeaves: want none, got %v", got)
	}
}
