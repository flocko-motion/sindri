package workflow

import (
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

func TestNeedsReview(t *testing.T) {
	for _, tc := range []struct {
		name string
		pr   store.PR
		live map[string]bool
		want bool
	}{
		{"open, no row", store.PR{ID: "pr-a", Status: "open"}, map[string]bool{}, true},
		{"open, live row", store.PR{ID: "pr-a", Status: "open"}, map[string]bool{"pr-a": true}, false},
		{"interim, no row", store.PR{ID: "pr-a", Status: "open", Kind: "interim"}, map[string]bool{}, false},
		{"not open", store.PR{ID: "pr-a", Status: "approved"}, map[string]bool{}, false},
	} {
		if got := needsReview(tc.pr, tc.live); got != tc.want {
			t.Errorf("%s: needsReview = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// pendingReview lays down the state the bug was found in: an open final PR whose review row was
// written while every reviewer was busy, so nobody was ever assigned to it.
func pendingReview(t *testing.T, reviewers ...string) (*store.Store, *store.ProjectStore) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	if err := ps.PutPR(store.PR{ID: "pr-waiting", Task: "td-1", Agent: "bombur", Branch: "sd-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.AddReview("pr-waiting", "check it"); err != nil {
		t.Fatal(err)
	}
	for _, r := range reviewers {
		if err := ps.PutAgent(store.Agent{Name: r, Role: "reviewer", Workspace: ".worktrees/" + r}); err != nil {
			t.Fatal(err)
		}
	}
	return st, ps
}

// reviewerOf returns who holds prID's review, "" while nobody does.
func reviewerOf(t *testing.T, ps *store.ProjectStore, prID string) string {
	t.Helper()
	revs, err := ps.Reviews(prID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range revs {
		if r.Verdict == "" {
			return r.Author
		}
	}
	return ""
}

// TestAnUnclaimedReviewReachesAnIdleReviewer is the fix for what left fili idle beside a PR waiting
// on it: the row was written unassigned because every reviewer was busy, and the only thing that
// ever claimed one was a reviewer choosing to ask. The hub now hands it over itself.
func TestAnUnclaimedReviewReachesAnIdleReviewer(t *testing.T) {
	st, ps := pendingReview(t, "fili")
	e := newEngine(st, &stubDeps{root: t.TempDir(), alive: true})

	e.AssignPendingReviews("repo")

	if got := reviewerOf(t, ps, "pr-waiting"); got != "fili" {
		t.Errorf("review of pr-waiting held by %q, want fili", got)
	}
	held, _ := ps.GetState("fili")
	if held.Phase != "reviewing" {
		t.Errorf("fili's phase = %q, want reviewing — the board must stop showing it idle", held.Phase)
	}
}

// TestAReviewerMidTurnIsLeftAlone is the whole reason the sweep asks the watchdog first. Injecting
// into a running turn is what failed before: the text lands in the input box and dies there when the
// turn ends, so the assignment would be recorded and the reviewer would never hear of it.
func TestAReviewerMidTurnIsLeftAlone(t *testing.T) {
	st, ps := pendingReview(t, "fili")
	e := newEngine(st, &stubDeps{root: t.TempDir(), alive: true, busy: map[string]bool{"fili": true}})

	e.AssignPendingReviews("repo")

	if got := reviewerOf(t, ps, "pr-waiting"); got != "" {
		t.Errorf("review assigned to %q mid-turn — it must wait for an idle prompt", got)
	}
}

// TestAReviewerAlreadyHoldingOneIsNotGivenASecond: a reviewer has one workspace, so it can hold
// exactly one PR — the rule RequestReview states and the sweep must not undercut by checking out a
// second branch over the one being read.
func TestAReviewerAlreadyHoldingOneIsNotGivenASecond(t *testing.T) {
	st, ps := pendingReview(t, "fili")
	if err := ps.PutPR(store.PR{ID: "pr-inhand", Task: "td-2", Agent: "dain", Branch: "sd-2", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	rid, err := ps.AddReview("pr-inhand", "in hand")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.AssignReview(rid, "fili"); err != nil {
		t.Fatal(err)
	}
	e := newEngine(st, &stubDeps{root: t.TempDir(), alive: true})

	e.AssignPendingReviews("repo")

	if got := reviewerOf(t, ps, "pr-waiting"); got != "" {
		t.Errorf("pr-waiting handed to %q, which is already reading another PR", got)
	}
}

// TestRetirementHoldsAcrossTheReviewQueueToo: a retired agent is handed no new work, and a queue
// nobody had swept was a way for work to reach one anyway.
func TestRetirementHoldsAcrossTheReviewQueueToo(t *testing.T) {
	st, ps := pendingReview(t)
	if err := ps.PutAgent(store.Agent{Name: "fili", Role: "reviewer", Workspace: ".worktrees/fili", Retired: true}); err != nil {
		t.Fatal(err)
	}
	e := newEngine(st, &stubDeps{root: t.TempDir(), alive: true})

	e.AssignPendingReviews("repo")

	if got := reviewerOf(t, ps, "pr-waiting"); got != "" {
		t.Errorf("a retired reviewer was handed %q", got)
	}
}

// TestRepairReviewRowsFindsAGap covers the backstop: an open final PR with no live review row —
// however it got that way — gets one, while an interim PR and one already covered are left alone.
func TestRepairReviewRowsFindsAGap(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("repo")
	for _, p := range []store.PR{
		{ID: "pr-gap", Task: "td-gap", Agent: "bombur", Branch: "pr-gap", Base: "main", Status: "open"},
		{ID: "pr-covered", Task: "td-covered", Agent: "bombur", Branch: "pr-covered", Base: "main", Status: "open"},
		{ID: "pr-milestone", Task: "td-mile", Agent: "bombur", Branch: "pr-milestone", Base: "main", Status: "open", Kind: "interim"},
	} {
		if err := ps.PutPR(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ps.AddReview("pr-covered", "already in hand"); err != nil {
		t.Fatal(err)
	}

	e := newEngine(st, &stubDeps{root: t.TempDir(), alive: true})
	e.RepairReviewRows("repo")

	gapRevs, _ := ps.Reviews("pr-gap")
	if len(gapRevs) == 0 {
		t.Error("pr-gap should have gained a review row")
	}
	coveredRevs, _ := ps.Reviews("pr-covered")
	if len(coveredRevs) != 1 {
		t.Errorf("pr-covered should keep its single row, got %d", len(coveredRevs))
	}
	milestoneRevs, _ := ps.Reviews("pr-milestone")
	if len(milestoneRevs) != 0 {
		t.Error("an interim (milestone/contribution) PR must never gain a review row")
	}
}
