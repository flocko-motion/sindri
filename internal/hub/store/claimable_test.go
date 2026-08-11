package store

import "testing"

// TestEveryOpenRatedUngatedUnheldTaskIsClaimable is the invariant OpenLeaves and OpenContainers
// must jointly satisfy: a task whose own status is open, whose own gate is clear, that carries a
// priority, and that no agent holds, is offered by exactly one of the two pools — never by
// neither. A task with real pending work still buried under a gate is the one deliberate
// exception: nothing is wrong with excluding it, because the gate clearing is what releases it
// later (-> TestAnEmptyPackageIsNotClaimable). Every other shape here has nothing left to wait
// on and must be offered NOW, or it is stuck for good.
func TestEveryOpenRatedUngatedUnheldTaskIsClaimable(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		// A standalone leaf: the base case both pools have always agreed on.
		{ID: "leaf-open", Status: "open", Priority: "P1"},

		// A container with one open, ungated child: the normal package shape.
		{ID: "pkg-working", Status: "open", Priority: "P1"},
		{ID: "pkg-working-c1", Status: "open", Priority: "P1", ParentID: "pkg-working"},

		// A container whose ONLY child has already closed, and whose own PR never merged (no PR
		// exists at all here) — the hole: OpenLeaves excludes it forever (it once had a child),
		// OpenContainers excluded it too (no OPEN child), and nothing else will ever touch its
		// status. It must be offered so someone can claim it and finish it.
		{ID: "pkg-stranded", Status: "open", Priority: "P2"},
		{ID: "pkg-stranded-c1", Status: "closed", Priority: "P1", ParentID: "pkg-stranded"},

		// Same shape, two levels deep: a grandchild closed, its parent epic auto-closed by
		// closeCompletedAncestors, leaving the TOP container the only one still open.
		{ID: "pkg-stranded-deep", Status: "open", Priority: "P2"},
		{ID: "pkg-stranded-deep-epic", Status: "closed", Priority: "P1", ParentID: "pkg-stranded-deep"},
		{ID: "pkg-stranded-deep-gc", Status: "closed", Priority: "P1", ParentID: "pkg-stranded-deep-epic"},

		// A container whose only child is gated: real work remains, just not actionable yet. This
		// one is the deliberate exception — excluded now, released once the gate clears.
		{ID: "pkg-gated", Status: "open", Priority: "P1"},
		{ID: "pkg-gated-c1", Status: "open", Priority: "P1", ParentID: "pkg-gated"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetApproval("pkg-gated-c1", "pending", ""); err != nil {
		t.Fatal(err)
	}

	claimable := map[string]bool{}
	for _, task := range mustLeaves(t, p) {
		claimable[task.ID] = true
	}
	for _, task := range mustContainers(t, p) {
		claimable[task.ID] = true
	}

	for _, want := range []string{"leaf-open", "pkg-working", "pkg-stranded", "pkg-stranded-deep"} {
		if !claimable[want] {
			t.Errorf("%s is open, rated, ungated, and unheld, with nothing left to wait on — but neither OpenLeaves nor OpenContainers offers it", want)
		}
	}
	// pkg-gated is excluded on purpose: its child's gate, not this invariant, releases it.
	if claimable["pkg-gated"] {
		t.Error("pkg-gated should stay excluded while its only child is gated")
	}
	// A task's children and gated-child are never themselves claimable units of work.
	for _, notAUnit := range []string{"pkg-working-c1", "pkg-stranded-c1", "pkg-stranded-deep-epic", "pkg-stranded-deep-gc", "pkg-gated-c1"} {
		if claimable[notAUnit] {
			t.Errorf("%s is a child worked inside its package, not a unit of its own — must not be offered directly", notAUnit)
		}
	}
}
