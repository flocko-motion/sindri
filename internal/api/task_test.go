package api

import "testing"

func TestArrangeTasksTree(t *testing.T) {
	tasks := []Task{
		{ID: "ep", Priority: "P1", Status: "open"},
		{ID: "f1", ParentID: "ep", Priority: "P1", Status: "open"},
		{ID: "t1", ParentID: "f1", Priority: "P2", Status: "open"},
		{ID: "f2", ParentID: "ep", Priority: "P2", Status: "open"},
		{ID: "orphan", ParentID: "ghost", Priority: "P3", Status: "open"}, // parent absent → root
		{ID: "bug", Priority: "P0", Status: "open"},                       // standalone, highest prio
	}
	prs := []PR{
		{ID: "pr-f1", Task: "f1", Status: "open", Kind: "interim"},
		{ID: "pr-x", Task: "t1", Status: "merged"},
		{ID: "pr-f2", Task: "f2", Status: "scrapped"}, // terminal → not annotated
	}

	rows := ArrangeTasks(tasks, prs)

	// Flatten to (id, depth) and check tree order + depth.
	type pair struct {
		id    string
		depth int
	}
	var got []pair
	for _, r := range rows {
		got = append(got, pair{r.ID, r.Depth})
	}
	// bug (P0 root) before ep (P1 root); ep's children nested; orphan a root last.
	want := []pair{
		{"bug", 0},
		{"ep", 0}, {"f1", 1}, {"t1", 2}, {"f2", 1},
		{"orphan", 0},
	}
	if len(got) != len(want) {
		t.Fatalf("rows: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("position %d: got %v want %v (full %v)", i, got[i], want[i], got)
		}
	}

	// PR annotation: f1 carries its open interim PR (id + kind); t1's is merged and
	// f2's is scrapped → neither is marked (both terminal).
	for _, r := range rows {
		switch r.ID {
		case "f1":
			if r.PR != "pr-f1" || r.PRKind != "interim" {
				t.Errorf("f1 should carry pr-f1/interim, got %q/%q", r.PR, r.PRKind)
			}
		case "t1":
			if r.PR != "" {
				t.Errorf("t1 PR is merged, should not be marked, got %q", r.PR)
			}
		case "f2":
			if r.PR != "" {
				t.Errorf("f2 PR is scrapped, should not be marked, got %q", r.PR)
			}
		}
	}
}
