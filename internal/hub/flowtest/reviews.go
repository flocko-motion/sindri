// package: hub/flowtest / reviews
// type:    assembly (a review row, as the hub would file it)
// job:     put one reviewer on one pull request — the roster row, the review row, and the claim —
// so a test about a VERDICT starts from a hold that really exists.
// limits:  seeding. Who may hold what is hub/flow/pr's rule, tested there.
package flowtest

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// AssignReviewer gives agent an Assigned (author-set, unverdicted) review row on pr — the shape
// CmdApprove's completeReview needs to find and stamp, without driving the full checkout/directive
// machinery reviewFixture's own reviewer already goes through.
func AssignReviewer(t *testing.T, ps *store.ProjectStore, pr, agent string) {
	t.Helper()
	if err := ps.PutAgent(store.Agent{Name: agent, Role: "reviewer", Workspace: ".worktrees/" + agent}); err != nil {
		t.Fatalf("put agent %s: %v", agent, err)
	}
	id, err := ps.AddReview(pr, "review it")
	if err != nil {
		t.Fatalf("add review: %v", err)
	}
	if err := ps.AssignReview(id, agent); err != nil {
		t.Fatalf("assign review: %v", err)
	}
}

// Retire sets an agent's retirement flag, which is what the hub's own retire verb writes.
func Retire(t *testing.T, ps *store.ProjectStore, name string) {
	t.Helper()
	ag, _, _ := ps.GetAgent(name)
	ag.Retired = true
	if err := ps.PutAgent(ag); err != nil {
		t.Fatal(err)
	}
}
