package store

import "testing"

// TestOpenChildIDsCountsAChildBeingWorked is what OpenChildIDs is FOR: it answers "would closing
// this parent be a lie", and it is the guard behind close, reconcile, checkpoint, submit and merge.
// It asked for status='open' literally, so a child an agent was actively working — in_progress —
// came back as no child at all, and the parent could be closed or merged over live work. That is
// the most severe form of the invariant, and the cheapest to reintroduce: the literal query looks
// tidier and nothing else here distinguishes the two.
func TestOpenChildIDsCountsAChildBeingWorked(t *testing.T) {
	p := openTmpProject(t)
	if err := p.ReplaceTasks([]Task{
		{ID: "td-parent", Status: "in_progress"},
		{ID: "td-working", Status: "in_progress", ParentID: "td-parent"},
		{ID: "td-waiting", Status: "open", ParentID: "td-parent"},
		{ID: "td-done", Status: "closed", ParentID: "td-parent"},
		{ID: "td-merged", Status: "merged", ParentID: "td-parent"},
		{ID: "td-approved", Status: "approved", ParentID: "td-parent"},
	}); err != nil {
		t.Fatal(err)
	}

	open, err := p.OpenChildIDs("td-parent")
	if err != nil {
		t.Fatalf("OpenChildIDs: %v", err)
	}
	got := map[string]bool{}
	for _, id := range open {
		got[id] = true
	}
	if !got["td-working"] {
		t.Errorf("a child being worked must count — closing over it is the worst case, got %v", open)
	}
	if !got["td-waiting"] {
		t.Errorf("a child not yet started must count too, got %v", open)
	}
	// The terminal statuses, and only those, are finished (-> api.DoneStatus).
	for _, done := range []string{"td-done", "td-merged", "td-approved"} {
		if got[done] {
			t.Errorf("%s has ended and must not hold its parent open, got %v", done, open)
		}
	}
}
