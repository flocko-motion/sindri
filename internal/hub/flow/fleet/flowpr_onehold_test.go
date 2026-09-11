package fleet

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// oneReviewerTwoPRs seeds a running reviewer and two open pull requests, each with a filed review
// row. Nothing is claimed: WHO holds what is the reviewer's own map to decide, and a fixture that
// wrote the hold itself could build a world the machine would never produce.
func oneReviewerTwoPRs(t *testing.T) (*Engine, *store.ProjectStore) {
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
	return newEngine(t, st, &flowtest.Hub{Root: root}), ps
}

// TestAReviewerHoldsExactlyOnePR is the constraint the workspace imposes: one checkout, so one PR.
// The directive used to scan the open PRs and serve whichever sorted first, which handed a reviewer
// a second while the first was still on disk — and the diff it read then matched neither.
func TestAReviewerHoldsExactlyOnePR(t *testing.T) {
	e, ps := oneReviewerTwoPRs(t)

	first, err := e.AgentDirective(t.Context(), "repo", "fili")
	if err != nil {
		t.Fatal(err)
	}
	held, _ := ps.ReviewingPR("fili")
	if held == "" {
		t.Fatal("taking a review must record the hold — that is what makes it exclusive")
	}
	if !strings.Contains(first, held) {
		t.Errorf("directive %q does not name the PR it holds (%s)", first, held)
	}
	// Asked again and again, it gets the SAME PR — the one whose branch is checked out.
	for range 3 {
		again, aerr := e.AgentDirective(t.Context(), "repo", "fili")
		if aerr != nil {
			t.Fatal(aerr)
		}
		if again != first {
			t.Fatalf("a held review must not change under the reviewer:\n first: %q\n now:   %q", first, again)
		}
	}
	// And a request arriving meanwhile is not forced onto it.
	if err := e.prAct().RequestReview("repo", "pr-b", "later"); err != nil {
		t.Fatalf("RequestReview: %v", err)
	}
	e.Look("repo", "fili")
	if now, _ := ps.ReviewingPR("fili"); now != held {
		t.Errorf("a busy reviewer was reassigned to %q — its workspace holds %q", now, held)
	}
}

// TestASettledPRReleasesItsReviewer: a merge overtakes any review still out on it. The reviewer is
// freed and told, rather than left holding a verdict that can no longer decide anything.
func TestASettledPRReleasesItsReviewer(t *testing.T) {
	e, ps := oneReviewerTwoPRs(t)
	e.Look("repo", "fili")
	held, _ := ps.ReviewingPR("fili")
	if held == "" {
		t.Fatal("setup: the reviewer should hold one of the two")
	}

	pr, _, _ := ps.GetPR(held)
	pr.Status = "merged"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}

	e.Look("repo", "fili")
	if now, _ := ps.ReviewingPR("fili"); now == held {
		t.Error("a merged PR must not still be held for review")
	}
}

// TestAmendingAReviewGoesToTheAgentOnIt: asking again while a reviewer holds the PR adds to what it
// was told, rather than opening a second review — it has the branch checked out, so the instructions
// belong to it. A second reviewer would check that branch out from under the first.
func TestAmendingAReviewGoesToTheAgentOnIt(t *testing.T) {
	e, ps := oneReviewerTwoPRs(t)
	e.Look("repo", "fili")
	held, _ := ps.ReviewingPR("fili")
	if held == "" {
		t.Fatal("setup: the reviewer should hold one of the two")
	}
	before, _ := ps.Reviews(held)

	if err := e.prAct().RequestReview("repo", held, "also check the error paths"); err != nil {
		t.Fatalf("RequestReview: %v", err)
	}

	after, _ := ps.Reviews(held)
	if len(after) != len(before) {
		t.Errorf("a second review was opened (%d -> %d) — the agent on it should just be told more",
			len(before), len(after))
	}
	var open int
	for _, r := range after {
		if r.Verdict != "" {
			continue
		}
		open++
		if r.Requirement != "also check the error paths" {
			t.Errorf("requirement = %q, want the new instructions", r.Requirement)
		}
		if r.Author != "fili" {
			t.Errorf("author = %q, want it to stay with fili", r.Author)
		}
	}
	if open != 1 {
		t.Errorf("%d open reviews, want exactly 1 — one reviewer, one review", open)
	}
}

// TestTheHandedDirectiveCarriesTheAuthor is the wiring, as opposed to the wording: what the reviewer
// is actually told must carry the PR's own author. It is the fact that makes the follow-up possible
// — a question to the person who wrote it, rather than a rejection written at nobody.
func TestTheHandedDirectiveCarriesTheAuthor(t *testing.T) {
	e, ps := oneReviewerTwoPRs(t)

	dir, err := e.AgentDirective(t.Context(), "repo", "fili")
	if err != nil {
		t.Fatal(err)
	}
	held, _ := ps.ReviewingPR("fili")
	pr, _, _ := ps.GetPR(held)
	if pr.Agent == "" {
		t.Fatal("precondition: the fixture's PRs have an author")
	}
	if !strings.Contains(dir, pr.Agent) {
		t.Errorf("the directive for %s does not name its author %q:\n%s", held, pr.Agent, dir)
	}
	// And again on the re-ask, which reads the HELD review rather than making a fresh claim — two
	// paths through the same words, one of which is easy to leave behind.
	again, err := e.AgentDirective(t.Context(), "repo", "fili")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(again, pr.Agent) {
		t.Errorf("the re-asked directive dropped the author:\n%s", again)
	}
}

// TestAReopenedPRIsClaimableAgain: a new request unmakes an approval, and the reviewer's own map
// picks the reopened pull request up — otherwise the approval would stand over instructions nobody
// had carried out.
func TestAReopenedPRIsClaimableAgain(t *testing.T) {
	e, ps := oneReviewerTwoPRs(t)
	pr, _, _ := ps.GetPR("pr-a")
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}

	if err := e.prAct().RequestReview("repo", "pr-a", "one more thing"); err != nil {
		t.Fatalf("RequestReview: %v", err)
	}

	if got, _, _ := ps.GetPR("pr-a"); got.Status != "open" {
		t.Errorf("status = %q, want open — a fresh question cannot sit behind an old answer", got.Status)
	}
	dir, err := e.AgentDirective(t.Context(), "repo", "fili")
	if err != nil {
		t.Fatal(err)
	}
	if held, _ := ps.ReviewingPR("fili"); held == "" {
		t.Fatalf("the reopened PR was not claimable: %q", dir)
	}
}
