package workflow

import (
	"io"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// assignReviewer gives agent an assigned (author-set, unverdicted) review row on pr — the shape
// CmdApprove's completeReview needs to find and stamp, without driving the full checkout/directive
// machinery reviewFixture's own reviewer already goes through.
func assignReviewer(t *testing.T, ps *store.ProjectStore, pr, agent string) {
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

// TestApprovalsAccumulateAcrossReviewers is the DONE WHEN case: two agents can both approve one
// PR, and both are recorded and visible with author and time — the first verdict no longer
// claims a single scalar that locks the second out.
func TestApprovalsAccumulateAcrossReviewers(t *testing.T) {
	e, ps, _ := reviewFixture(t)
	assignReviewer(t, ps, "pr-a", "fili")
	assignReviewer(t, ps, "pr-a", "kili")

	var out strings.Builder
	if code, err := e.CmdApprove(registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}, []string{"pr-a"}, &out); err != nil || code != 0 {
		t.Fatalf("fili approve: code=%d err=%v out=%s", code, err, out.String())
	}
	if pr, _, _ := ps.GetPR("pr-a"); pr.Status != "approved" {
		t.Fatalf("status after first approval = %q, want approved", pr.Status)
	}

	out.Reset()
	if code, err := e.CmdApprove(registry.Caller{Project: "repo", Agent: "kili", Role: "reviewer"}, []string{"pr-a"}, &out); err != nil || code != 0 {
		t.Fatalf("kili's approval of an already-approved PR should accumulate, not be refused: code=%d err=%v out=%s", code, err, out.String())
	}
	if pr, _, _ := ps.GetPR("pr-a"); pr.Status != "approved" {
		t.Errorf("status after second approval = %q, want still approved", pr.Status)
	}

	revs, err := ps.Reviews("pr-a")
	if err != nil {
		t.Fatalf("reviews: %v", err)
	}
	if n := api.ApprovalCount(revs); n != 2 {
		t.Fatalf("approval count = %d, want 2: %+v", n, revs)
	}
	seen := map[string]bool{}
	for _, r := range revs {
		if r.Verdict != "pass" {
			continue
		}
		seen[r.Author] = true
		if r.VerdictAt == "" {
			t.Errorf("approval by %s has no VerdictAt — 'when' is part of the badge", r.Author)
		}
	}
	if !seen["fili"] || !seen["kili"] {
		t.Errorf("recorded approvers = %v, want both fili and kili", seen)
	}
}

// TestRejectionDominatesRegardlessOfApprovalCount: two standing approvals do not outrun a
// rejection, and a fresh approval attempt cannot clear it either — only a renewed submission can.
func TestRejectionDominatesRegardlessOfApprovalCount(t *testing.T) {
	e, ps, _ := reviewFixture(t)
	assignReviewer(t, ps, "pr-a", "fili")
	assignReviewer(t, ps, "pr-a", "kili")
	for _, agent := range []string{"fili", "kili"} {
		if code, err := e.CmdApprove(registry.Caller{Project: "repo", Agent: agent, Role: "reviewer"}, []string{"pr-a"}, io.Discard); err != nil || code != 0 {
			t.Fatalf("%s approve: code=%d err=%v", agent, code, err)
		}
	}
	if pr, _, _ := ps.GetPR("pr-a"); pr.Status != "approved" {
		t.Fatalf("setup: status = %q, want approved before the rejection", pr.Status)
	}

	if err := e.RejectPR("repo", "pr-a", "not like this"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if pr, _, _ := ps.GetPR("pr-a"); pr.Status != "rejected" {
		t.Errorf("status = %q, want rejected — a rejection must dominate two standing approvals", pr.Status)
	}
	// The two approvals are not erased — they stay as history, just no longer sufficient.
	revs, _ := ps.Reviews("pr-a")
	if n := api.ApprovalCount(revs); n != 2 {
		t.Errorf("approval count after rejection = %d, want the 2 approvals preserved as history", n)
	}
	// And a fresh approval cannot overturn the rejection; only renewal (resolve.go) does.
	if err := e.ApprovePR("repo", "pr-a"); err == nil {
		t.Error("approving a rejected PR must be refused — the way back is renewal, not another approval")
	}
}

// TestSettledPRRefusesNewApprovals is the approve-side twin of TestAVerdictOnLandedWorkIsRefused:
// a merged or scrapped PR must keep refusing verdicts even now that approvals accumulate.
func TestSettledPRRefusesNewApprovals(t *testing.T) {
	for _, settled := range []string{"merged", "scrapped"} {
		e, ps, _ := reviewFixture(t)
		pr, _, _ := ps.GetPR("pr-a")
		pr.Status = settled
		if err := ps.PutPR(pr); err != nil {
			t.Fatal(err)
		}
		if err := e.ApprovePR("repo", "pr-a"); err == nil {
			t.Errorf("approving a %s PR must be refused", settled)
		}
		if got, _, _ := ps.GetPR("pr-a"); got.Status != settled {
			t.Errorf("status = %q — a refused approval must not rewrite it", got.Status)
		}
	}
}

// TestPlannerBadgeIsAdvisoryAndNeverSatisfiesMergeAlone covers the other half of sd-121327: a
// planner's approval is additional and optional, recorded and visible, but it must never by
// itself move a PR to "approved" — only a reviewer's (or a human's) verdict opens the gate.
func TestPlannerBadgeIsAdvisoryAndNeverSatisfiesMergeAlone(t *testing.T) {
	e, ps, _ := reviewFixture(t)
	if err := ps.PutAgent(store.Agent{Name: "opsx", Role: "planner", Workspace: ".worktrees/opsx"}); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if code, err := e.CmdApprove(registry.Caller{Project: "repo", Agent: "opsx", Role: "planner"}, []string{"pr-a"}, &out); err != nil || code != 0 {
		t.Fatalf("planner approve: code=%d err=%v out=%s", code, err, out.String())
	}
	if pr, _, _ := ps.GetPR("pr-a"); pr.Status == "approved" {
		t.Error("a planner's badge alone must never move the PR to approved")
	}
	revs, _ := ps.Reviews("pr-a")
	var badge *store.Review
	for i, r := range revs {
		if r.Author == "opsx" && r.Verdict == "pass" {
			badge = &revs[i]
		}
	}
	if badge == nil {
		t.Fatal("the planner's approval was not recorded")
	}
	if !badge.Advisory {
		t.Error("a planner's badge must be marked Advisory")
	}
	if badge.VerdictAt == "" {
		t.Error("the planner's badge has no VerdictAt")
	}

	// A real reviewer's approval alongside it is what actually opens the gate — additional, not
	// a replacement: both badges stand once it lands.
	if _, ok, err := e.reviewDirective(t.Context(), "repo", "fili"); err != nil || !ok {
		t.Fatalf("reviewDirective: ok=%v err=%v", ok, err)
	}
	if code, err := e.CmdApprove(registry.Caller{Project: "repo", Agent: "fili", Role: "reviewer"}, []string{"pr-a"}, io.Discard); err != nil || code != 0 {
		t.Fatalf("reviewer approve: code=%d err=%v", code, err)
	}
	if pr, _, _ := ps.GetPR("pr-a"); pr.Status != "approved" {
		t.Errorf("status = %q, want approved once the reviewer weighs in beside the planner's badge", pr.Status)
	}
	revs, _ = ps.Reviews("pr-a")
	if n := api.ApprovalCount(revs); n != 2 {
		t.Errorf("approval count = %d, want 2 (planner + reviewer badges both counted)", n)
	}
}
