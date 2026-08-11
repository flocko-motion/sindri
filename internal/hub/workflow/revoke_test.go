package workflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// revokeFixture seeds a worker that has submitted, with a reviewer holding the PR.
func revokeFixture(t *testing.T, prStatus string) (*Engine, *store.ProjectStore, registry.Caller) {
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
	if err := ps.PutAgent(store.Agent{Name: "nidi", Role: "worker", Workspace: ".worktrees/nidi"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{
		ID: "pr-gh-285", Task: "gh-285", Agent: "nidi", Branch: "gh-285", Base: "main", Status: prStatus,
	}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{
		Agent: "nidi", Task: "gh-285", Branch: "gh-285", Phase: "submitted",
	}); err != nil {
		t.Fatal(err)
	}
	return New(st, &stubDeps{root: root, alive: true}),
		ps, registry.Caller{Project: "repo", Agent: "nidi", Role: "worker", Phase: "submitted"}
}

// TestRevokeHandsTheTaskBack is the way out that did not exist: an agent that realises mid-review
// its work is incomplete withdraws its own PR and carries on, rather than waiting for a verdict on
// something it already knows is wrong — and since submit is the only thing that commits, waiting is
// how its later changes ended up recorded nowhere at all.
func TestRevokeHandsTheTaskBack(t *testing.T) {
	e, ps, c := revokeFixture(t, "open")
	var out strings.Builder
	if code, err := e.CmdRevoke(c, []string{"the translation change needs a test first"}, &out); err != nil || code != 0 {
		t.Fatalf("CmdRevoke: code=%d err=%v out=%s", code, err, out.String())
	}

	// Back at work on the same task AND the same branch: the point is to keep what is already on it.
	st, _ := ps.GetState("nidi")
	if st.Phase != "working" || st.Task != "gh-285" || st.Branch != "gh-285" {
		t.Errorf("state = {phase:%q task:%q branch:%q}, want working on gh-285", st.Phase, st.Task, st.Branch)
	}
	// The PR is off the table but its history is kept, with who withdrew it and why.
	pr, _, _ := ps.GetPR("pr-gh-285")
	if pr.Status != "rejected" {
		t.Errorf("PR status = %q, want it out of the running", pr.Status)
	}
	if !strings.Contains(pr.Feedback, "nidi") || !strings.Contains(pr.Feedback, "needs a test") {
		t.Errorf("feedback = %q, want the author and the reason", pr.Feedback)
	}
	if !strings.Contains(out.String(), "gh-285") {
		t.Errorf("the reply should name what came back: %s", out.String())
	}
}

// TestRevokeReleasesTheReviewer: whoever was reading it is reading a branch about to change.
func TestRevokeReleasesTheReviewer(t *testing.T) {
	e, ps, c := revokeFixture(t, "open")
	id, err := ps.AddReview("pr-gh-285", "look at it")
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.AssignReview(id, "fili"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CmdRevoke(c, nil, &strings.Builder{}); err != nil {
		t.Fatalf("CmdRevoke: %v", err)
	}
	if held, _ := ps.ReviewingPR("fili"); held != "" {
		t.Errorf("fili still holds %q — the branch is about to change under it", held)
	}
}

// TestRevokeWorksOnAnApprovedPR: an approval is not a reason to merge something its own author says
// is unfinished. If anyone objects, least of all the person who wrote it, it does not go in.
func TestRevokeWorksOnAnApprovedPR(t *testing.T) {
	e, ps, c := revokeFixture(t, "approved")
	if code, err := e.CmdRevoke(c, []string{"found a gap"}, &strings.Builder{}); err != nil || code != 0 {
		t.Fatalf("revoking an approved PR must work: code=%d err=%v", code, err)
	}
	if pr, _, _ := ps.GetPR("pr-gh-285"); pr.Status == "approved" {
		t.Error("an approved PR its author withdrew must not stay merge-ready")
	}
}

// TestRevokeWithNothingOutSaysSo: no PR, nothing withdrawn — and it says where the agent actually
// stands rather than reporting a success that did not happen.
func TestRevokeWithNothingOutSaysSo(t *testing.T) {
	e, ps, c := revokeFixture(t, "merged") // settled: not a live PR
	var out strings.Builder
	code, err := e.CmdRevoke(c, nil, &out)
	if err != nil || code == 0 {
		t.Fatalf("want a non-zero exit and no error, got code=%d err=%v", code, err)
	}
	if !strings.Contains(out.String(), "Nothing to withdraw") {
		t.Errorf("out = %q, want it to say nothing was withdrawn", out.String())
	}
	if pr, _, _ := ps.GetPR("pr-gh-285"); pr.Status != "merged" {
		t.Errorf("PR status = %q — a merged PR cannot be withdrawn", pr.Status)
	}
}
