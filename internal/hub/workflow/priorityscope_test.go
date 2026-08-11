package workflow

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// priorityTree builds an engine over an epic with a rated child, an unrated grandchild, an unrated
// child, a closed child, and a task outside the tree.
func priorityTree(t *testing.T) (*Engine, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("proj", t.TempDir()); err != nil {
		t.Fatalf("register: %v", err)
	}
	ps := st.For("proj")
	// Seeded the way the hub stores a task it owns: the authoritative owned row, the read model the
	// claim queries read, and parentage in its own table — a rating re-reads all three (-> RefreshTask).
	for _, x := range []store.Task{
		{ID: "td-epic", Status: "open", Title: "epic"},
		{ID: "td-a", Status: "open", ParentID: "td-epic", Title: "a", Priority: "P0"},
		{ID: "td-deep", Status: "open", ParentID: "td-a", Title: "deep"},
		{ID: "td-b", Status: "open", ParentID: "td-epic", Title: "b"},
		{ID: "td-done", Status: "closed", ParentID: "td-epic", Title: "done"},
		{ID: "td-other", Status: "open", Title: "other"},
	} {
		if err := ps.PutOwnedTask(store.OwnedTask{ID: x.ID, Title: x.Title, Status: x.Status, Priority: x.Priority}); err != nil {
			t.Fatalf("seed %s: %v", x.ID, err)
		}
		if err := ps.UpsertTask(x); err != nil {
			t.Fatalf("mirror %s: %v", x.ID, err)
		}
		if err := ps.SetParent(x.ID, x.ParentID); err != nil {
			t.Fatalf("link %s: %v", x.ID, err)
		}
	}
	return New(st, &stubDeps{root: t.TempDir()}), ps
}

// priorityOf reads a task's priority back off the store.
func priorityOf(t *testing.T, ps *store.ProjectStore, id string) string {
	t.Helper()
	got, ok, err := ps.GetTask(id)
	if err != nil || !ok {
		t.Fatalf("get %s: %v (found %v)", id, err, ok)
	}
	return got.Priority
}

// TestScopeTaskLeavesTheTreeAlone is the unchanged behaviour, and the one the store's own semantics
// depend on: rating an epic releases the package, and its children are MEANT to come along unrated.
func TestScopeTaskLeavesTheTreeAlone(t *testing.T) {
	e, ps := priorityTree(t)
	if err := e.SetPriority("proj", "td-epic", "P2", api.ScopeTask); err != nil {
		t.Fatalf("SetPriority: %v", err)
	}
	if got := priorityOf(t, ps, "td-epic"); got != "P2" {
		t.Errorf("td-epic = %q, want P2", got)
	}
	for _, id := range []string{"td-deep", "td-b"} {
		if got := priorityOf(t, ps, id); got != "" {
			t.Errorf("%s = %q, want it left unrated", id, got)
		}
	}
}

// TestScopeUnratedSpareTheDeliberateOnes: "all unrated children" is a fill-in-the-blanks, so a child
// somebody rated on purpose keeps that rating — otherwise the narrower scope would be a wider one.
func TestScopeUnratedSparesTheDeliberateOnes(t *testing.T) {
	e, ps := priorityTree(t)
	if err := e.SetPriority("proj", "td-epic", "P2", api.ScopeUnrated); err != nil {
		t.Fatalf("SetPriority: %v", err)
	}
	if got := priorityOf(t, ps, "td-a"); got != "P0" {
		t.Errorf("td-a = %q, want its own P0 kept", got)
	}
	for _, id := range []string{"td-epic", "td-deep", "td-b"} {
		if got := priorityOf(t, ps, id); got != "P2" {
			t.Errorf("%s = %q, want P2", id, got)
		}
	}
}

// TestScopeAllOverwritesButNotThePast: "all children" means all of them, at any depth — except the
// ones that have ended. A finished task's rating decides nothing, and rewriting it would edit the
// record of work already done.
func TestScopeAllOverwritesButNotThePast(t *testing.T) {
	e, ps := priorityTree(t)
	if err := e.SetPriority("proj", "td-epic", "P3", api.ScopeAll); err != nil {
		t.Fatalf("SetPriority: %v", err)
	}
	for _, id := range []string{"td-epic", "td-a", "td-deep", "td-b"} {
		if got := priorityOf(t, ps, id); got != "P3" {
			t.Errorf("%s = %q, want P3", id, got)
		}
	}
	if got := priorityOf(t, ps, "td-done"); got != "" {
		t.Errorf("td-done = %q — a closed task must not be re-rated", got)
	}
	if got := priorityOf(t, ps, "td-other"); got != "" {
		t.Errorf("td-other = %q — it is outside the tree", got)
	}
}

// TestAClosedParentsChildrenBecomeClaimable is the trap the scopes exist for. A parent closing over
// open children turns each into a standalone leaf, and a leaf with no priority is offered to nobody —
// which is exactly how approved work sat in this backlog unclaimable, with nothing on screen saying
// why. Carrying the rating down is what releases them, and the assigner's own query is the proof.
func TestAClosedParentsChildrenBecomeClaimable(t *testing.T) {
	e, ps := priorityTree(t)
	if err := ps.SetOwnedStatus("td-epic", "closed"); err != nil {
		t.Fatal(err)
	}
	if err := ps.UpsertTask(store.Task{ID: "td-epic", Status: "closed", Title: "epic"}); err != nil {
		t.Fatal(err)
	}
	if got := taskIDs(t, ps.OpenLeaves); len(got) != 0 {
		t.Fatalf("before rating, nothing under the closed epic can be claimed; got %v", got)
	}
	if err := e.SetPriority("proj", "td-epic", "P1", api.ScopeUnrated); err != nil {
		t.Fatalf("SetPriority: %v", err)
	}
	// td-b now stands alone and rated, so the assigner offers it. td-deep does not appear here and
	// must not: its own parent td-a is open, which makes td-a a package and td-deep its subtask.
	if got := taskIDs(t, ps.OpenLeaves); len(got) != 1 || got[0] != "td-b" {
		t.Errorf("claimable leaves = %v, want [td-b]", got)
	}
	if got := taskIDs(t, ps.OpenContainers); len(got) != 1 || got[0] != "td-a" {
		t.Errorf("claimable packages = %v, want [td-a]", got)
	}
}

// taskIDs names what a claim query returned, failing the test if the query itself did.
func taskIDs(t *testing.T, query func() ([]store.Task, error)) []string {
	t.Helper()
	ts, err := query()
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(ts))
	for i, task := range ts {
		out[i] = task.ID
	}
	return out
}
