package task

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// ids joins a task set's ids, for comparing an expected walk in one string.
func ids(ts []store.Task) string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}
	return strings.Join(out, ",")
}

// TestDescendantsDeepestFirst: the whole subtree comes back — grandchildren included,
// done ones included — with every child ahead of its parent, which is what lets a
// cascading scrap delete each task while its own children are already gone.
func TestDescendantsDeepestFirst(t *testing.T) {
	tasks := []store.Task{
		{ID: "td-1"},
		{ID: "td-2", ParentID: "td-1"},
		{ID: "td-3", ParentID: "td-2", Status: "closed"},
		{ID: "td-4", ParentID: "td-1"},
		{ID: "td-9"}, // an unrelated root
	}
	if got, want := ids(Descendants(tasks, "td-1")), "td-3,td-2,td-4"; got != want {
		t.Errorf("Descendants(td-1) = %q, want %q", got, want)
	}
	if got := ids(Descendants(tasks, "td-4")); got != "" {
		t.Errorf("a leaf has no descendants, got %q", got)
	}
	if got := ids(Descendants(tasks, "td-none")); got != "" {
		t.Errorf("an unknown id has no descendants, got %q", got)
	}
}

// TestDescendantsSurvivesAParentLoop: a parent chain that points back into itself must
// be walked once, not forever — the walk drives a delete, so a hang here would wedge
// the hub on a malformed task set from a backend.
func TestDescendantsSurvivesAParentLoop(t *testing.T) {
	tasks := []store.Task{
		{ID: "td-1", ParentID: "td-2"},
		{ID: "td-2", ParentID: "td-1"},
		{ID: "td-3", ParentID: "td-3"},
	}
	if got, want := ids(Descendants(tasks, "td-1")), "td-2"; got != want {
		t.Errorf("Descendants(td-1) = %q, want %q", got, want)
	}
	if got := ids(Descendants(tasks, "td-3")); got != "" {
		t.Errorf("a task parented to itself is not its own descendant, got %q", got)
	}
}
