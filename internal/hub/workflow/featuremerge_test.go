package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestMergingAFinishedFeatureReleasesTheWorker is what a merge is FOR. The merge path had one branch
// for a held feature — land it, keep the branch, put the agent back on it — written when the only
// way a feature reached a PR was a partial milestone the user cut mid-flight. A feature's own final
// PR took that branch too, so merging it closed nothing and freed nobody: the worker stayed assigned
// to a feature already in the reference branch, and its task still read open.
func TestMergingAFinishedFeatureReleasesTheWorker(t *testing.T) {
	e, ps, c, _ := featureWorker(t, false) // no open subtasks: this PR is the feature's last
	var out strings.Builder
	if code, err := e.CmdSubmit(c, []string{"the feature"}, &out); err != nil || code != 0 {
		t.Fatalf("CmdSubmit: code=%d err=%v out=%s", code, err, out.String())
	}
	runQueuedGate(t, e)
	pr, ok, _ := ps.GetPR("pr-td-EPIC")
	if !ok {
		t.Fatal("the feature should be up for review")
	}
	pr.Status = "approved"
	if err := ps.PutPR(pr); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Merge("repo", pr.ID); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// Released: no task, no feature, resting — free to take whatever is next.
	st, _ := ps.GetState("dain")
	if st.Container != "" || st.Task != "" || st.Phase != "idle" {
		t.Errorf("after its feature merged the worker holds {task:%q feature:%q phase:%q}, want nothing",
			st.Task, st.Container, st.Phase)
	}
	// And the feature itself is finished, not left open behind a merged PR.
	if owned, ok, _ := ps.OwnedTask("td-EPIC"); ok && owned.Status != "closed" {
		t.Errorf("td-EPIC status = %q after its PR merged, want closed", owned.Status)
	}
}

// TestAPartialMilestoneKeepsTheWorkerOnTheFeature is the other half, and the reason the branch above
// exists: cutting a milestone with subtasks still open lands the work so far and leaves the worker
// exactly where it was, on the feature branch, carrying on.
func TestAPartialMilestoneKeepsTheWorkerOnTheFeature(t *testing.T) {
	e, ps, _, _ := featureWorker(t, true) // a subtask still open
	if err := ps.PutPR(store.PR{
		ID: "pr-td-EPIC", Task: "td-EPIC", Agent: "dain", Branch: "td-EPIC", Base: "main", Status: "approved",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Merge("repo", "pr-td-EPIC"); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	st, _ := ps.GetState("dain")
	if st.Container != "td-EPIC" {
		t.Errorf("a partial milestone must leave the worker on its feature, got %q", st.Container)
	}
	if owned, ok, _ := ps.OwnedTask("td-EPIC"); ok && owned.Status == "closed" {
		t.Error("a partial milestone must not close the feature — its subtasks are not done")
	}
}

// TestAPartialMilestoneKeepsUncommittedWorkAcrossTheReset guards the reset the milestone path uses
// to bring a standing branch onto its new base: unlike ResetBranchTo (built to discard, for scrap),
// it must carry the agent's own uncommitted edits and untracked scratch across the move rather
// than wiping them along with the branch's stale pre-squash commits.
func TestAPartialMilestoneKeepsUncommittedWorkAcrossTheReset(t *testing.T) {
	e, ps, _, deps := featureWorker(t, true) // a subtask still open — the branch stays standing
	wt := filepath.Join(deps.root, ".worktrees", "dain")
	// featureWorker already left feature.txt sitting untracked; edit a TRACKED file too, uncommitted.
	if err := os.WriteFile(filepath.Join(wt, "seed"), []byte("mid-edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{
		ID: "pr-td-EPIC", Task: "td-EPIC", Agent: "dain", Branch: "td-EPIC", Base: "main", Status: "approved",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Merge("repo", "pr-td-EPIC"); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(wt, "seed")); err != nil || string(got) != "mid-edit\n" {
		t.Errorf("uncommitted edit to a tracked file lost across the reset: got %q, err %v", got, err)
	}
	if _, err := os.ReadFile(filepath.Join(wt, "feature.txt")); err != nil {
		t.Errorf("untracked scratch file lost across the reset: %v", err)
	}
}

// TestAWorkerIsNeverHandedALandedFeature: whatever route left it holding one, an agent whose feature
// has already merged is released on its next ask rather than being sent round the loop again.
func TestAWorkerIsNeverHandedALandedFeature(t *testing.T) {
	e, ps, _, _ := featureWorker(t, false)
	// The shape the old merge path left behind: PR merged, feature still open, worker still on it.
	if err := ps.PutPR(store.PR{
		ID: "pr-td-EPIC", Task: "td-EPIC", Agent: "dain", Branch: "td-EPIC", Status: "merged",
	}); err != nil {
		t.Fatal(err)
	}
	// Released, it falls through to the ordinary wait for work — which blocks, correctly, since the
	// backlog is empty. The bounded context is what ends the call; the state is the assertion.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	dir, _ := e.AgentDirective(ctx, "repo", "dain")
	if strings.Contains(dir, "td-EPIC") {
		t.Errorf("a merged feature must not be handed back: %q", dir)
	}
	if st, _ := ps.GetState("dain"); st.Container != "" {
		t.Errorf("the merged feature should have been released, still holding %q", st.Container)
	}
}
