package store

import "testing"

// TestAnEmptyPackageIsNotClaimable: a package is offered because an agent can be put to work inside
// it, so "has an open child" is the wrong test — that child may itself be gated, or be a parent of
// gated work. Two packages sat in the claimable pool for a day this way, handed out and immediately
// returning nothing, while the queries behind the offer and the claim disagreed about the same tree.
func TestAnEmptyPackageIsNotClaimable(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "td-gated", Status: "open", Priority: "P1"},
		{ID: "td-child", Status: "open", ParentID: "td-gated"}, // its only work, held back below
		{ID: "td-ready", Status: "open", Priority: "P1"},
		{ID: "td-work", Status: "open", ParentID: "td-ready"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetApproval("td-child", "pending", ""); err != nil {
		t.Fatal(err)
	}

	got := ids(mustContainers(t, p))
	if len(got) != 1 || got[0] != "td-ready" {
		t.Fatalf("claimable packages = %v, want [td-ready] — td-gated has nothing to hand out", got)
	}
	// Whatever OpenContainers offers, claiming it must find work: that is the invariant.
	for _, c := range mustContainers(t, p) {
		if len(mustSubtasks(t, p, c.ID)) == 0 {
			t.Errorf("%s is offered as claimable but has no workable subtask", c.ID)
		}
	}

	// Ruling on the gate is what releases it — nothing else about the tree changed.
	if err := p.SetApproval("td-child", "approved", ""); err != nil {
		t.Fatal(err)
	}
	if got := ids(mustContainers(t, p)); len(got) != 2 {
		t.Errorf("both packages should be claimable once the gate clears, got %v", got)
	}
}
