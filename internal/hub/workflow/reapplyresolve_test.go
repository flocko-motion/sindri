package workflow

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/store"
)

// forceReapplyConflict puts wt into the exact shape a milestone reset's clashing reapply leaves
// (-> git.ResetOntoKeepingWork): unmerged files, no rebase in progress. RebaseStep's guarantee
// that a branch is fast-forward ahead of its base at squash time makes that shape unreachable
// through a live Merge() call — the squash reproduces the branch's own pre-merge tree exactly, so
// a stashed diff can never conflict reapplying onto it (git_test.go covers the mechanism directly).
// So this drives ResetOntoKeepingWork with a deliberately mismatched target instead, to exercise
// what CmdResolve does with the state Merge() would otherwise have left.
func forceReapplyConflict(t *testing.T, wt, branch, trackedFile string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", wt}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run("checkout", "-qb", "elsewhere", branch)
	if err := os.WriteFile(filepath.Join(wt, trackedFile), []byte("elsewhere version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// -a, not -A: only trackedFile's own modification is committed, never a fixture's stray
	// untracked scratch file (e.g. featureWorker's feature.txt), which would otherwise ride along.
	run("commit", "-qam", "elsewhere change")
	run("checkout", "-q", branch)
	if err := os.WriteFile(filepath.Join(wt, trackedFile), []byte("agent's uncommitted work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	conflicts, done, err := git.ResetOntoKeepingWork(wt, "elsewhere")
	if err != nil {
		t.Fatalf("ResetOntoKeepingWork: %v", err)
	}
	if done || len(conflicts) == 0 {
		t.Fatalf("setup must produce a clashing reapply, got conflicts=%v done=%v", conflicts, done)
	}
}

// TestResolveAfterReapplyConflictResumesInterimContribution is shape (a): a plain interim
// contribution's PR is keyed by its own task, and the merged PR must stay merged — never renewed
// or sent up for a review nobody is waiting to give, and the worker returns to "working" its task.
func TestResolveAfterReapplyConflictResumesInterimContribution(t *testing.T) {
	e, ps, c, deps := leafWorker(t, "resolving")
	wt := filepath.Join(deps.root, ".worktrees", "dain")
	forceReapplyConflict(t, wt, "td-LEAF", "seed")
	if err := ps.PutPR(store.PR{ID: "pr-td-LEAF", Task: "td-LEAF", Agent: "dain", Branch: "td-LEAF", Base: "main", Status: "merged", Kind: "interim"}); err != nil {
		t.Fatal(err)
	}
	// leafWorker already left the agent's state at {Task: td-LEAF, Branch: td-LEAF, Phase: resolving}.
	// The worker resolves the markers, exactly as `sindri resolve`'s reply instructs.
	if err := os.WriteFile(filepath.Join(wt, "seed"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if code, err := e.CmdResolve(c, nil, io.Discard); code != 0 || err != nil {
		t.Fatalf("CmdResolve: code=%d err=%v", code, err)
	}

	if pr, ok, _ := ps.GetPR("pr-td-LEAF"); !ok || pr.Status != "merged" {
		t.Errorf("a merged PR must not be renewed by resolving a post-merge reapply, got status %q", pr.Status)
	}
	st, _ := ps.GetState("dain")
	if st.Phase != "working" || st.Task != "td-LEAF" {
		t.Errorf("the agent should resume working td-LEAF, got {phase:%q task:%q}", st.Phase, st.Task)
	}
	if !containsSubstring(deps.injectedText, "pr-td-LEAF") {
		t.Error("the agent should have been told its contribution merged")
	}
}

// TestResolveAfterReapplyConflictResumesAnEstablishedFeature is shape (b): the milestone PR is
// keyed by the CONTAINER, not the subtask st.Task holds while resolving — and with that subtask
// still open, resumeContainer must leave the worker on it rather than starting it over.
func TestResolveAfterReapplyConflictResumesAnEstablishedFeature(t *testing.T) {
	e, ps, c, deps := featureWorker(t, true) // td-1 still open
	wt := filepath.Join(deps.root, ".worktrees", "dain")
	forceReapplyConflict(t, wt, "td-EPIC", "seed")
	if err := ps.PutPR(store.PR{ID: "pr-td-EPIC", Task: "td-EPIC", Agent: "dain", Branch: "td-EPIC", Base: "main", Status: "merged", Kind: "interim"}); err != nil {
		t.Fatal(err)
	}
	// The shape workflow/merge.go's own reset step leaves: Task is the real subtask, not pr.Task.
	if err := ps.SetState(store.AgentState{Agent: "dain", Task: "td-1", Branch: "td-EPIC", Container: "td-EPIC", Phase: "resolving"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "seed"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if code, err := e.CmdResolve(c, nil, io.Discard); code != 0 || err != nil {
		t.Fatalf("CmdResolve: code=%d err=%v", code, err)
	}

	if pr, ok, _ := ps.GetPR("pr-td-EPIC"); !ok || pr.Status != "merged" {
		t.Errorf("a merged milestone PR must not be renewed, got status %q", pr.Status)
	}
	st, _ := ps.GetState("dain")
	if st.Container != "td-EPIC" || st.Task != "td-1" || st.Phase != "working" {
		t.Errorf("resumeContainer should keep the still-open subtask, got {container:%q task:%q phase:%q}",
			st.Container, st.Task, st.Phase)
	}
}

// TestResolveAfterReapplyConflictResumesAJustPromotedFeature is shape (c): promoteToFeature
// leaves Task empty (no subtask assigned yet), so resumeContainer must ADVANCE the container to
// one instead of parking the agent on an empty task.
func TestResolveAfterReapplyConflictResumesAJustPromotedFeature(t *testing.T) {
	e, ps, c, deps := featureWorker(t, true) // td-1 open, ready for advanceContainer to pick
	wt := filepath.Join(deps.root, ".worktrees", "dain")
	forceReapplyConflict(t, wt, "td-EPIC", "seed")
	if err := ps.PutPR(store.PR{ID: "pr-td-EPIC", Task: "td-EPIC", Agent: "dain", Branch: "td-EPIC", Base: "main", Status: "merged", Kind: "interim"}); err != nil {
		t.Fatal(err)
	}
	// promoteToFeature's own shape (-> adopt.go): Container set, Task left empty.
	if err := ps.SetState(store.AgentState{Agent: "dain", Task: "", Branch: "td-EPIC", Container: "td-EPIC", Phase: "resolving"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "seed"), []byte("resolved\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if code, err := e.CmdResolve(c, nil, io.Discard); code != 0 || err != nil {
		t.Fatalf("CmdResolve: code=%d err=%v", code, err)
	}

	if pr, ok, _ := ps.GetPR("pr-td-EPIC"); !ok || pr.Status != "merged" {
		t.Errorf("a merged milestone PR must not be renewed, got status %q", pr.Status)
	}
	st, _ := ps.GetState("dain")
	if st.Container != "td-EPIC" || st.Task == "" {
		t.Errorf("a just-promoted feature must advance to a subtask, got {container:%q task:%q}", st.Container, st.Task)
	}
}

// containsSubstring reports whether any of texts contains sub.
func containsSubstring(texts []string, sub string) bool {
	for _, txt := range texts {
		if strings.Contains(txt, sub) {
			return true
		}
	}
	return false
}
