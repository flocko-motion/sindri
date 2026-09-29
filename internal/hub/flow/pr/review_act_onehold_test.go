package pr

import (
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// reviewFixture seeds a running reviewer and two open PRs awaiting review.
func reviewFixture(t *testing.T) (*Act, *store.ProjectStore, *flowtest.Hub) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	root := t.TempDir()
	if err := st.RegisterProject("repo", root); err != nil {
		t.Fatal(err)
	}
	ps := st.For("repo")
	if err := ps.PutAgent(store.Agent{Name: "fili", Role: "reviewer", Workspace: ".worktrees/fili"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"pr-a", "pr-b"} {
		if err := ps.PutPR(store.PR{
			ID: id, Task: "td-" + id, Agent: "bombur", Branch: id, Base: "main", Status: "open",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := ps.AddReview(id, "review it"); err != nil {
			t.Fatal(err)
		}
	}
	return newActOn(t, st, &flowtest.Hub{Root: root}), ps, &flowtest.Hub{}
}

// TestAVerdictOnLandedWorkIsRefused is the write that undid a merge: a reviewer rejected an
// already-merged PR, the record read "rejected", its author was sent back to a landed branch, and
// the two looped on an empty diff three times over. Only LANDED work refuses a verdict.
func TestAVerdictOnLandedWorkIsRefused(t *testing.T) {
	for _, settled := range []string{"merged", "scrapped"} {
		a, ps, _ := reviewFixture(t)
		pr, _, _ := ps.GetPR("pr-a")
		pr.Status = settled
		if err := ps.PutPR(pr); err != nil {
			t.Fatal(err)
		}
		err := a.RejectPR("repo", "pr-a", "too late")
		if err == nil {
			t.Fatalf("rejecting a %s PR must be refused", settled)
		}
		if !strings.Contains(err.Error(), settled) {
			t.Errorf("the refusal should say what state it is in: %v", err)
		}
		if got, _, _ := ps.GetPR("pr-a"); got.Status != settled {
			t.Errorf("status = %q — a refused verdict must not rewrite it", got.Status)
		}
	}
}

// TestAnApprovedPRCanStillBeRejected: approval is the state BEFORE a merge, not an outcome. A
// reviewer sets it, and overruling that to stop something merging is what a human verdict is for —
// so this is the one transition the settled-work guard must not catch.
func TestAnApprovedPRCanStillBeRejected(t *testing.T) {
	a, ps, deps := reviewFixture(t)
	pr, _, _ := ps.GetPR("pr-a")
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	if err := a.RejectPR("repo", "pr-a", "not going in like that"); err != nil {
		t.Fatalf("rejecting an approved PR must be allowed: %v", err)
	}
	got, _, _ := ps.GetPR("pr-a")
	if got.Status != "rejected" {
		t.Errorf("status = %q, want rejected", got.Status)
	}
	if got.Feedback != "not going in like that" {
		t.Errorf("feedback = %q, want the reason to reach the author", got.Feedback)
	}
	// And the work comes back to the author, which is what its own map reads to put it on the next
	// round (-> cond.Rejected). The verdict moves nobody itself.
	if st, _ := ps.GetState("bombur"); st.Task != got.Task {
		t.Errorf("author holds %q, want the rejected work %q back", st.Task, got.Task)
	}
	_ = deps
}

// TestANewRequestUnmakesAnApproval: the verdict answered the previous question. Asking a new one
// reopens the PR, or nothing could be handed the reviewer and the approval would stand over
// instructions nobody had carried out.
func TestANewRequestUnmakesAnApproval(t *testing.T) {
	a, ps, _ := reviewFixture(t)
	pr, _, _ := ps.GetPR("pr-a")
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	if err := a.RequestReview("repo", "pr-a", "one more thing"); err != nil {
		t.Fatalf("RequestReview: %v", err)
	}
	got, _, _ := ps.GetPR("pr-a")
	if got.Status != "open" {
		t.Errorf("status = %q, want open — a fresh question cannot sit behind an old answer", got.Status)
	}
	// The new requirement is on the row a reviewer will be handed, which is what reopening is FOR.
	revs, _ := ps.Reviews("pr-a")
	var open int
	for _, r := range revs {
		if r.Verdict == "" && r.Requirement == "one more thing" {
			open++
		}
	}
	if open != 1 {
		t.Errorf("%d open rows carrying the new requirement, want exactly 1", open)
	}
}

// TestAMergedPRIsNotReopenedByAReviewRequest: only an approval is unmade. Merged and scrapped are
// settled for good, and reopening one would put a reviewer on work already in the reference branch.
func TestAMergedPRIsNotReopenedByAReviewRequest(t *testing.T) {
	a, ps, _ := reviewFixture(t)
	pr, _, _ := ps.GetPR("pr-a")
	pr.Status = "merged"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	if err := a.RequestReview("repo", "pr-a", "have another look"); err != nil {
		t.Fatalf("RequestReview: %v", err)
	}
	if got, _, _ := ps.GetPR("pr-a"); got.Status != "merged" {
		t.Errorf("status = %q — a merged PR stays merged", got.Status)
	}
}

// hold puts one reviewer on one pull request through the ONE function that assigns a review — the
// machine's own action half. A test in this package cannot run the machine, so it calls what the
// machine calls rather than writing the row itself.
func hold(t *testing.T, a *Act, ps *store.ProjectStore, prID, reviewer string) {
	t.Helper()
	if _, ok, _ := ps.GetAgent(reviewer); !ok {
		if err := ps.PutAgent(store.Agent{Name: reviewer, Role: "reviewer", Workspace: ".worktrees/" + reviewer}); err != nil {
			t.Fatal(err)
		}
	}
	id, err := ps.AddReview(prID, "review it")
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := a.AssignReview(t.Context(), "repo", id, prID, reviewer, "review it")
	if err != nil || !claimed {
		t.Fatalf("AssignReview(%s -> %s): claimed=%v err=%v", prID, reviewer, claimed, err)
	}
}
