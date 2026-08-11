package hub

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestTheBoardSaysWhoIsReviewing: whether a PR is being looked at, and by whom, is the hub's answer.
// A front-end deriving it from the review records would be deciding rather than rendering, and the
// two would disagree the first time the rule moved — so it arrives on the PR itself.
func TestTheBoardSaysWhoIsReviewing(t *testing.T) {
	h := newHub(t)
	ps := h.store.For(testProject)
	for _, id := range []string{"pr-watched", "pr-alone"} {
		if err := ps.PutPR(store.PR{ID: id, Task: "td-1", Agent: "bombur", Branch: id, Status: "open"}); err != nil {
			t.Fatal(err)
		}
	}
	id, err := ps.AddReview("pr-watched", "look at it")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.AssignReview(id, "fili"); err != nil {
		t.Fatal(err)
	}
	// A review already answered says nothing about who is reviewing NOW.
	done, err := ps.AddReview("pr-alone", "looked at it")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.AssignReview(done, "fili"); err != nil {
		t.Fatal(err)
	}
	if err := ps.RecordVerdict(done, "pass", "fine"); err != nil {
		t.Fatal(err)
	}

	prs := []store.PR{
		{Project: testProject, ID: "pr-watched"},
		{Project: testProject, ID: "pr-alone"},
	}
	h.fillReviewers(prs)
	if prs[0].Reviewer != "fili" {
		t.Errorf("pr-watched reviewer = %q, want fili", prs[0].Reviewer)
	}
	if prs[1].Reviewer != "" {
		t.Errorf("pr-alone reviewer = %q, want empty — its review is finished", prs[1].Reviewer)
	}
}
