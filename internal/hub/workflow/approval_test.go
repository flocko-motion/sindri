package workflow

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// approvalTree builds an engine over a throwaway store holding an epic with four children, each in
// a different approval state — the shapes a cascading approve has to tell apart.
func approvalTree(t *testing.T) (*Engine, *store.ProjectStore) {
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
	// The epic carries a priority so the claimability check has everything but the gate.
	tasks := []struct{ id, status, parent, approval string }{
		{"td-epic", "open", "", "pending"},
		{"td-a", "open", "td-epic", "pending"},
		{"td-b", "open", "td-a", "pending"}, // a grandchild: the tree can nest
		{"td-no", "open", "td-epic", "rejected"},
		{"td-done", "closed", "td-epic", "pending"},
		{"td-other", "open", "", "pending"}, // outside the tree
	}
	for _, x := range tasks {
		prio := ""
		if x.id == "td-epic" {
			prio = "P1"
		}
		if err := ps.UpsertTask(store.Task{ID: x.id, Status: x.status, ParentID: x.parent, Title: x.id, Priority: prio}); err != nil {
			t.Fatalf("upsert %s: %v", x.id, err)
		}
		if err := ps.SetApproval(x.id, x.approval, ""); err != nil {
			t.Fatalf("gate %s: %v", x.id, err)
		}
	}
	return newEngine(st, &stubDeps{root: t.TempDir()}), ps
}

// gateOf reads a task's approval back off the store.
func gateOf(t *testing.T, ps *store.ProjectStore, id string) string {
	t.Helper()
	got, _ := ps.GetApproval(id)
	return got
}

// TestApproveSubtreeReleasesThePendingOnes is what the modal offers: one verdict for the package.
// A rejected child and a closed one keep their state — the first is a verdict already given, the
// second decides nothing — and a task outside the tree is never touched.
func TestApproveSubtreeReleasesThePendingOnes(t *testing.T) {
	e, ps := approvalTree(t)
	if err := e.ApproveTask("proj", "td-epic", true); err != nil {
		t.Fatalf("ApproveTask: %v", err)
	}
	for _, want := range []struct{ id, gate string }{
		{"td-epic", "approved"},
		{"td-a", "approved"},
		{"td-b", "approved"}, // nested, so the walk has to go deeper than one level
		{"td-no", "rejected"},
		{"td-done", "pending"},
		{"td-other", "pending"},
	} {
		if got := gateOf(t, ps, want.id); got != want.gate {
			t.Errorf("%s: gate = %q, want %q", want.id, got, want.gate)
		}
	}
}

// TestApproveWithoutSubtreeLeavesTheChildrenGated keeps the narrow verdict narrow: the modal's
// "this task only" must decide nothing below it.
func TestApproveWithoutSubtreeLeavesTheChildrenGated(t *testing.T) {
	e, ps := approvalTree(t)
	if err := e.ApproveTask("proj", "td-epic", false); err != nil {
		t.Fatalf("ApproveTask: %v", err)
	}
	if got := gateOf(t, ps, "td-epic"); got != "approved" {
		t.Errorf("td-epic: gate = %q, want approved", got)
	}
	for _, id := range []string{"td-a", "td-b"} {
		if got := gateOf(t, ps, id); got != "pending" {
			t.Errorf("%s: gate = %q, want pending", id, got)
		}
	}
}

// TestApprovedPackageIsClaimableAsOne is the point of the cascade: with the whole tree released,
// the epic is a claimable package and its children are the work inside it. Approving the epic
// alone leaves the store with a package it cannot hand out.
func TestApprovedPackageIsClaimableAsOne(t *testing.T) {
	for _, tc := range []struct {
		subtree bool
		want    int
	}{{false, 0}, {true, 1}} {
		e, ps := approvalTree(t)
		if err := e.ApproveTask("proj", "td-epic", tc.subtree); err != nil {
			t.Fatalf("subtree=%v: %v", tc.subtree, err)
		}
		containers, err := ps.OpenContainers()
		if err != nil {
			t.Fatal(err)
		}
		if tc.subtree {
			if len(containers) != 1 {
				t.Fatalf("subtree=true: want the epic claimable, got %d containers", len(containers))
			}
			// td-b, not td-a: this fixture's tree nests, and td-a is a parent of an open child, so
			// the work inside the package is the leaf beneath it. Serving td-a here is what let a
			// checkpoint close it over td-b.
			children, err := ps.OpenSubtasks("td-epic")
			if err != nil {
				t.Fatal(err)
			}
			if len(children) != 1 || children[0].ID != "td-b" {
				t.Errorf("work inside a released package: got %d task(s), want [td-b]", len(children))
			}
			continue
		}
		// Narrow approve: the epic has no approved open child, so there is nothing to work.
		children, err := ps.OpenSubtasks("td-epic")
		if err != nil {
			t.Fatal(err)
		}
		if len(children) != tc.want {
			t.Errorf("subtree=false: want no claimable children, got %d", len(children))
		}
	}
}
