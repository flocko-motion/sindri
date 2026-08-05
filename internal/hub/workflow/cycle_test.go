package workflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// chainEngine seeds tasks a…d and parents them into whatever chain the links describe.
func chainEngine(t *testing.T, links map[string]string, ids ...string) (*Engine, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("proj", root); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("proj")
	for _, id := range ids {
		if err := ps.PutOwnedTask(store.OwnedTask{ID: id, Title: id, Status: "open"}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
		// The read model is what checkParent consults for existence.
		if err := ps.UpsertTask(store.Task{ID: id, Title: id, Status: "open"}); err != nil {
			t.Fatalf("mirror %s: %v", id, err)
		}
	}
	for child, parent := range links {
		if err := ps.SetParent(child, parent); err != nil {
			t.Fatalf("link %s under %s: %v", child, parent, err)
		}
	}
	return New(st, &stubDeps{root: root}), ps
}

// TestCycleThroughThreeTasksIsRefused is the case that slipped through: only self-parenting was
// checked, so a→b→c→a could be written, and a loop is unreachable from any root — every task inside
// it drops off the list.
func TestCycleThroughThreeTasksIsRefused(t *testing.T) {
	// b under a, c under b. Parenting a under c would close the loop.
	e, ps := chainEngine(t, map[string]string{"td-b": "td-a", "td-c": "td-b"}, "td-a", "td-b", "td-c")
	err := e.EditTask("proj", "td-a", TaskSpec{Parent: "td-c"})
	if err == nil {
		t.Fatal("parenting a task under its own descendant must be refused")
	}
	for _, want := range []string{"td-a", "td-c", "loop"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should name %q so it can be understood: %v", want, err)
		}
	}
	// Nothing may have been written: the check runs before the link.
	if got := ps.ParentOf("td-a"); got != "" {
		t.Errorf("td-a gained parent %q despite the refusal", got)
	}
}

// TestDirectCycleIsRefused: the two-task case, a→b→a.
func TestDirectCycleIsRefused(t *testing.T) {
	e, ps := chainEngine(t, map[string]string{"td-b": "td-a"}, "td-a", "td-b")
	if err := e.EditTask("proj", "td-a", TaskSpec{Parent: "td-b"}); err == nil {
		t.Fatal("a→b→a must be refused")
	}
	if got := ps.ParentOf("td-a"); got != "" {
		t.Errorf("td-a gained parent %q despite the refusal", got)
	}
}

// TestSelfParentingIsRefused keeps the case that always worked.
func TestSelfParentingIsRefused(t *testing.T) {
	e, _ := chainEngine(t, nil, "td-a")
	if err := e.EditTask("proj", "td-a", TaskSpec{Parent: "td-a"}); err == nil {
		t.Fatal("a task must not be its own parent")
	}
}

// TestALegitimateReParentStillWorks, or the guard would just forbid the hierarchy it protects: d is
// unrelated to the a→b→c chain, so hanging it under c is fine.
func TestALegitimateReParentStillWorks(t *testing.T) {
	e, ps := chainEngine(t, map[string]string{"td-b": "td-a", "td-c": "td-b"},
		"td-a", "td-b", "td-c", "td-d")
	if err := e.EditTask("proj", "td-d", TaskSpec{Parent: "td-c"}); err != nil {
		t.Fatalf("hanging an unrelated task under a deep chain: %v", err)
	}
	if got := ps.ParentOf("td-d"); got != "td-c" {
		t.Errorf("parent of td-d = %q, want td-c", got)
	}
}

// TestCyclesAcrossTaskTypesAreRefused: the hierarchy spans every kind, so the walk has to as well —
// an openspec change in the middle of the chain cannot hide a loop.
func TestCyclesAcrossTaskTypesAreRefused(t *testing.T) {
	e, _ := chainEngine(t, map[string]string{"os-mid": "td-a", "gh-9": "os-mid"},
		"td-a", "os-mid", "gh-9")
	if err := e.EditTask("proj", "td-a", TaskSpec{Parent: "gh-9"}); err == nil {
		t.Fatal("a loop running through an openspec change and a GitHub issue must be refused")
	}
}

// TestAnUnknownParentIsRefused before any walk: a typo names nothing to walk from.
func TestAnUnknownParentIsRefused(t *testing.T) {
	e, _ := chainEngine(t, nil, "td-a")
	if err := e.EditTask("proj", "td-a", TaskSpec{Parent: "td-nope"}); err == nil {
		t.Fatal("an unknown parent must be refused")
	}
}

// TestAStoredLoopIsReportedRatherThanSpun: data written before this check, or by hand, must not hang
// the walk. Bounded, and it says what it found.
func TestAStoredLoopIsReportedRatherThanSpun(t *testing.T) {
	e, ps := chainEngine(t, map[string]string{"td-b": "td-a"}, "td-a", "td-b", "td-c")
	// Close a loop behind the guard's back, as only a direct store write can.
	if err := ps.SetParent("td-a", "td-b"); err != nil {
		t.Fatal(err)
	}
	err := e.EditTask("proj", "td-c", TaskSpec{Parent: "td-a"})
	if err == nil {
		t.Fatal("walking into a stored loop must be reported")
	}
	if !strings.Contains(err.Error(), "doesn't terminate") {
		t.Errorf("the report should name the cause: %v", err)
	}
}
