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
	children, err := p.OpenChildren("td-123")
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(children); len(got) != 3 {
		t.Fatalf("OpenChildren: want all three unrated children, got %v", got)
	}
	// And no child is offered on its own: that would split the tree across agents.
	if got := mustLeaves(t, p); len(got) != 0 {
		t.Fatalf("OpenLeaves: want none, got %v", got)
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
