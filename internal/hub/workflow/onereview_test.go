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
