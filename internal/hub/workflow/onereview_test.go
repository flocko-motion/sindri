package workflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// reviewFixture seeds a running reviewer and two open PRs awaiting review.
func reviewFixture(t *testing.T) (*Engine, *store.ProjectStore, *stubDeps) {
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
	return New(st, &stubDeps{root: root, alive: true}), ps, &stubDeps{}
}

// TestAReviewerHoldsExactlyOnePR is the constraint the workspace imposes: one checkout, so one PR.
// The directive used to scan the open PRs and serve whichever sorted first, which handed a reviewer
// a second while the first was still on disk — and the diff it read then matched neither.
func TestAReviewerHoldsExactlyOnePR(t *testing.T) {
	e, ps, _ := reviewFixture(t)

	first, ok, err := e.reviewDirective("repo", "fili")
	if err != nil || !ok {
		t.Fatalf("reviewDirective: ok=%v err=%v", ok, err)
	}
	held, _ := ps.ReviewingPR("fili")
	if held == "" {
		t.Fatal("taking a review must record the hold — that is what makes it exclusive")
	}
	if !strings.Contains(first, held) {
		t.Errorf("directive %q does not name the PR it holds (%s)", first, held)
	}
	// Asked again and again, it gets the SAME PR — the one whose branch is checked out.
	for i := 0; i < 3; i++ {
		again, ok, err := e.reviewDirective("repo", "fili")
		if err != nil || !ok {
			t.Fatalf("reviewDirective: ok=%v err=%v", ok, err)
		}
		if again != first {
			t.Fatalf("a held review must not change under the reviewer:\n first: %q\n now:   %q", first, again)
		}
	}
	// And a request arriving meanwhile is not forced onto it.
	if err := e.RequestReview("repo", "pr-b", "later"); err != nil {
		t.Fatalf("RequestReview: %v", err)
	}
	if now, _ := ps.ReviewingPR("fili"); now != held {
		t.Errorf("a busy reviewer was reassigned to %q — its workspace holds %q", now, held)
	}
}

// TestASettledPRReleasesItsReviewer: a merge overtakes any review still out on it. The reviewer is
// freed and told, rather than left holding a verdict that can no longer decide anything.
func TestASettledPRReleasesItsReviewer(t *testing.T) {
	e, ps, _ := reviewFixture(t)
	if _, ok, err := e.reviewDirective("repo", "fili"); err != nil || !ok {
		t.Fatalf("reviewDirective: ok=%v err=%v", ok, err)
	}
	held, _ := ps.ReviewingPR("fili")

	pr, _, _ := ps.GetPR(held)
	pr.Status = "merged"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	// Its next ask releases the moot review and moves it to the other PR.
	dir, ok, err := e.reviewDirective("repo", "fili")
	if err != nil {
		t.Fatalf("reviewDirective: %v", err)
	}
	if now, _ := ps.ReviewingPR("fili"); now == held {
		t.Error("a merged PR must not still be held for review")
	}
	if ok && strings.Contains(dir, held) {
		t.Errorf("the settled PR was handed back: %q", dir)
	}
}

// TestAVerdictOnASettledPRIsRefused is the write that undid a merge: a reviewer rejected an
// already-merged PR, the record read "rejected", its author was sent back to a landed branch, and
// the two looped on an empty diff three times over.
func TestAVerdictOnASettledPRIsRefused(t *testing.T) {
	e, ps, _ := reviewFixture(t)
	pr, _, _ := ps.GetPR("pr-a")
	pr.Status = "merged"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	err := e.RejectPR("repo", "pr-a", "too late")
	if err == nil {
		t.Fatal("rejecting a merged PR must be refused")
	}
	if !strings.Contains(err.Error(), "merged") {
		t.Errorf("the refusal should say what state it is in: %v", err)
	}
	if got, _, _ := ps.GetPR("pr-a"); got.Status != "merged" {
		t.Errorf("status = %q — a refused verdict must not rewrite it", got.Status)
	}
}

// TestAmendingAReviewGoesToTheAgentOnIt: asking again while a reviewer holds the PR adds to what it
// was told, rather than opening a second review — it has the branch checked out, so the instructions
// belong to it. A second reviewer would check that branch out from under the first.
func TestAmendingAReviewGoesToTheAgentOnIt(t *testing.T) {
	e, ps, _ := reviewFixture(t)
	if _, ok, err := e.reviewDirective("repo", "fili"); err != nil || !ok {
		t.Fatalf("reviewDirective: ok=%v err=%v", ok, err)
	}
	held, _ := ps.ReviewingPR("fili")
	before, _ := ps.Reviews(held)

	if err := e.RequestReview("repo", held, "also check the error paths"); err != nil {
		t.Fatalf("RequestReview: %v", err)
	}
	after, _ := ps.Reviews(held)
	if len(after) != len(before) {
		t.Errorf("a second review was opened (%d -> %d) — the agent on it should just be told more",
			len(before), len(after))
	}
	var open int
	for _, r := range after {
		if r.Verdict == "" {
			open++
			if r.Requirement != "also check the error paths" {
				t.Errorf("requirement = %q, want the new instructions", r.Requirement)
			}
			if r.Author != "fili" {
				t.Errorf("author = %q, want it to stay with fili", r.Author)
			}
		}
	}
	if open != 1 {
		t.Errorf("%d open reviews, want exactly 1 — one reviewer, one review", open)
	}
	if now, _ := ps.ReviewingPR("fili"); now != held {
		t.Errorf("the hold moved to %q; it must stay on %q", now, held)
	}
}

// TestANewRequestUnmakesAnApproval: the verdict answered the previous question. Asking a new one
// reopens the PR, or nothing could be handed the reviewer and the approval would stand over
// instructions nobody had carried out.
func TestANewRequestUnmakesAnApproval(t *testing.T) {
	e, ps, _ := reviewFixture(t)
	pr, _, _ := ps.GetPR("pr-a")
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	if err := e.RequestReview("repo", "pr-a", "one more thing"); err != nil {
		t.Fatalf("RequestReview: %v", err)
	}
	got, _, _ := ps.GetPR("pr-a")
	if got.Status != "open" {
		t.Errorf("status = %q, want open — a fresh question cannot sit behind an old answer", got.Status)
	}
	// And it is now claimable again, with the new requirement.
	dir, ok, err := e.reviewDirective("repo", "fili")
	if err != nil || !ok {
		t.Fatalf("reviewDirective after reopening: ok=%v err=%v", ok, err)
	}
	if !strings.Contains(dir, "pr-a") {
		t.Errorf("directive = %q, want the reopened PR", dir)
	}
}

// TestAMergedPRIsNotReopenedByAReviewRequest: only an approval is unmade. Merged and scrapped are
// settled for good, and reopening one would put a reviewer on work already in the reference branch.
func TestAMergedPRIsNotReopenedByAReviewRequest(t *testing.T) {
	e, ps, _ := reviewFixture(t)
	pr, _, _ := ps.GetPR("pr-a")
	pr.Status = "merged"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	if err := e.RequestReview("repo", "pr-a", "have another look"); err != nil {
		t.Fatalf("RequestReview: %v", err)
	}
	if got, _, _ := ps.GetPR("pr-a"); got.Status != "merged" {
		t.Errorf("status = %q — a merged PR stays merged", got.Status)
	}
}
