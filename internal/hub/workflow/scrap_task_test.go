package workflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestScrapTaskGoesDeepestFirst: a subtree scrap must reach the grandchild before the
// child and the child before the task, so no backend is ever asked to delete a parent
// that still has children under it. The ids belong to no backend, so every scrap fails
// the same way — which is exactly what makes the order readable: the error names the
// task it got to first, and nothing above it was touched.
func TestScrapTaskGoesDeepestFirst(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("proj")
	for _, tk := range []store.Task{
		{ID: "zz-1", Title: "parent"},
		{ID: "zz-2", Title: "child", ParentID: "zz-1"},
		{ID: "zz-3", Title: "grandchild", ParentID: "zz-2"},
	} {
		if err := ps.UpsertTask(tk); err != nil {
			t.Fatal(err)
		}
	}
	e := newEngine(st, &stubDeps{root: t.TempDir()})

	err = e.ScrapTask("proj", "zz-1", true, false)
	if err == nil || !strings.Contains(err.Error(), "zz-3") {
		t.Fatalf("the deepest task must be scrapped first, so the failure names zz-3: %v", err)
	}
	// And the run stopped there: a partial scrap leaves a smaller tree, never a
	// subtask orphaned under a task that is already gone.
	for _, id := range []string{"zz-1", "zz-2", "zz-3"} {
		if _, ok, _ := ps.GetTask(id); !ok {
			t.Errorf("%s should still stand after the scrap aborted", id)
		}
	}
}

// TestScrapTaskWithoutSubtreeLeavesTheChildren: the default scrap is still the
// one-task discard — the children go only when the caller asks for them.
func TestScrapTaskWithoutSubtreeLeavesTheChildren(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("proj")
	if err := ps.UpsertTask(store.Task{ID: "zz-1"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "zz-2", ParentID: "zz-1"}); err != nil {
		t.Fatal(err)
	}
	e := newEngine(st, &stubDeps{root: t.TempDir()})

	err = e.ScrapTask("proj", "zz-1", false, false)
	if err == nil || !strings.Contains(err.Error(), "zz-1") {
		t.Fatalf("a plain scrap must act on the task itself: %v", err)
	}
	if strings.Contains(err.Error(), "zz-2") {
		t.Errorf("the child must not be touched without subtree: %v", err)
	}
}
