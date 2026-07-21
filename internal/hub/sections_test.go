package hub

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

func TestSectionCounts(t *testing.T) {
	b := BoardState{
		Tasks: []store.Task{
			{ID: "a", Status: "open"}, {ID: "b", Status: "in_progress"},
			{ID: "c", Status: "closed"}, {ID: "d", Status: "merged"},
		},
		Agents: []AgentView{{Name: "x", Status: "idle"}, {Name: "y", Status: "down"}},
		PRs:    []store.PR{{ID: "p1", Status: "open"}, {ID: "p2", Status: "merged"}, {ID: "p3", Status: "scrapped"}},
	}
	want := map[string]int{"tasks": 2, "agents": 2, "prs": 1} // non-closed; whole roster; open only (merged AND scrapped excluded)
	for _, s := range Sections {
		if got := s.Count(b); got != want[s.Key] {
			t.Errorf("%s count = %d, want %d", s.Key, got, want[s.Key])
		}
	}
}

func TestArrangeTasksTree(t *testing.T) {
	tasks := []store.Task{
		{ID: "ep", Priority: "P1", Status: "open"},
		{ID: "f1", ParentID: "ep", Priority: "P1", Status: "open"},
		{ID: "t1", ParentID: "f1", Priority: "P2", Status: "open"},
		{ID: "f2", ParentID: "ep", Priority: "P2", Status: "open"},
		{ID: "orphan", ParentID: "ghost", Priority: "P3", Status: "open"}, // parent absent → root
		{ID: "bug", Priority: "P0", Status: "open"},                       // standalone, highest prio
	}
	prs := []store.PR{{ID: "pr-f1", Task: "f1", Status: "open"}, {ID: "pr-x", Task: "t1", Status: "merged"}}

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

	// PR annotation: f1 has a non-merged PR; t1's PR is merged → not marked.
	for _, r := range rows {
		switch r.ID {
		case "f1":
			if r.PR != "pr-f1" {
				t.Errorf("f1 should carry pr-f1, got %q", r.PR)
			}
		case "t1":
			if r.PR != "" {
				t.Errorf("t1 PR is merged, should not be marked, got %q", r.PR)
			}
		}
	}
}
