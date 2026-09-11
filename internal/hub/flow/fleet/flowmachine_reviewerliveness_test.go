package fleet

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/reviewer"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// sleepingReviewers is a repo with one review waiting and reviewers whose pods are all down —
// the world that stranded dvalin's PR on 2026-09-10.
func sleepingReviewers(t *testing.T, names ...string) (*store.Store, *store.ProjectStore, *flowtest.Hub) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.RegisterProject("repo", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "td-1", Agent: "bombur", Branch: "sd-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddReview("pr-1", "review it"); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if err := ps.PutAgent(store.Agent{Name: n, Role: "reviewer", Workspace: ".worktrees/" + n, Stopped: true}); err != nil {
			t.Fatal(err)
		}
	}
	return st, ps, &flowtest.Hub{Root: t.TempDir(), Down: true}
}

// unclaimed reports whether the review row is still waiting for somebody to take it.
func unclaimed(t *testing.T, ps *store.ProjectStore) bool {
	t.Helper()
	var id int64
	var prID string
	found, err := ps.UnclaimedReview(&id, &prID)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// TestADeadReviewerIsNeverHandedAWaitingReview is the failure this change was found through: a PR
// was filed while every reviewer's pod was down, and the machine's own beat walked all four into
// `reviewer/clearing` on a condition that asks whether a row is waiting and never whether the
// reviewer is alive. The clear failed against a dead pod, the map led to `taking` regardless, and
// the claim erased the unclaimed row that both wake paths key on.
func TestADeadReviewerIsNeverHandedAWaitingReview(t *testing.T) {
	st, ps, d := sleepingReviewers(t, "balin", "dwalin")
	e := newEngine(t, st, d)

	e.LookProject("repo")

	if !unclaimed(t, ps) {
		t.Error("a reviewer with no pod took the review — the row is claimed and nothing is left saying a reviewer was needed")
	}
	for _, name := range []string{"balin", "dwalin"} {
		st, err := ps.GetState(name)
		if err != nil {
			t.Fatal(err)
		}
		if st.Phase == reviewer.Reviewing || st.Phase == reviewer.Taking || st.Phase == reviewer.Clearing {
			t.Errorf("%s stands in %s with its pod down — nothing may be handed to a dead agent", name, st.Phase)
		}
	}
}

// TestAWaitingReviewStartsAReviewerBeforeAnythingIsClaimed: reclaiming an idle reviewer's pod is
// only half a rule, and waking one is the other half. The machine starts a stopped reviewer and
// the row stays waiting until it is up, so the evidence a reviewer was needed outlives the wake.
func TestAWaitingReviewStartsAReviewerBeforeAnythingIsClaimed(t *testing.T) {
	st, ps, d := sleepingReviewers(t, "balin")
	e := newEngine(t, st, d)

	e.LookProject("repo")

	if len(d.Started) == 0 {
		t.Fatal("no reviewer was started — a review waiting on an empty pool must bring one back")
	}
	if d.Started[0] != "balin" {
		t.Errorf("started %q, want balin — the one stopped reviewer", d.Started[0])
	}
	if !unclaimed(t, ps) {
		t.Error("the review was claimed on the way past — the row is what says a reviewer is needed")
	}
}
