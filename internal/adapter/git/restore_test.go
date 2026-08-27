package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMergeBaseAgreesWithGitsOwnThreeDot: BranchDiff/Diff use base...branch, which git resolves to
// this same commit internally — the whole point of naming it explicitly is that RestoreFromRef can
// target the identical commit a three-dot diff already measures against.
func TestMergeBaseAgreesWithGitsOwnThreeDot(t *testing.T) {
	repo := newRepo(t)
	base, _ := CurrentBranch(repo)
	if err := CreateBranch(repo, "feat", base); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, repo, "feat.go", "feature")
	if err := CommitAll(repo, "feat"); err != nil {
		t.Fatal(err)
	}
	mustCommitOn(t, repo, base, "base.go", "base-moved") // the reference advances independently

	got, err := MergeBase(repo, base, "feat")
	if err != nil {
		t.Fatalf("MergeBase: %v", err)
	}
	want := strings.TrimSpace(gitOut(t, repo, "merge-base", base, "feat"))
	if got != want {
		t.Errorf("MergeBase = %q, want %q (git's own answer)", got, want)
	}
	if got == strings.TrimSpace(gitOut(t, repo, "rev-parse", base)) {
		t.Error("the reference advanced after the branch forked — merge-base must not be its current tip")
	}
}

// TestCommittedChurnIgnoresTheWorktree is the fix for the regression a worktree-only no-op check
// invited: a caller must be able to tell "the branch's own commits already agree with ref" from "the
// worktree happens to look that way right now", because RestoreFromRef and CommitAll answer the
// first question, not the second — a hand-edit back to ref's content, left uncommitted, must not
// read as settled while HEAD still carries the committed churn.
func TestCommittedChurnIgnoresTheWorktree(t *testing.T) {
	repo := newRepo(t)
	original := strings.TrimSpace(gitOut(t, repo, "rev-parse", "HEAD"))
	if churn, err := CommittedChurn(repo, original, []string{"f"}); err != nil || churn {
		t.Fatalf("an untouched file has no committed churn against its own commit: churn=%v err=%v", churn, err)
	}
	mustWrite(t, repo, "f", "changed")
	if err := CommitAll(repo, "diverge"); err != nil {
		t.Fatal(err)
	}
	if churn, err := CommittedChurn(repo, original, []string{"f"}); err != nil || !churn {
		t.Fatalf("a committed edit is churn against the original commit regardless of the worktree: churn=%v err=%v", churn, err)
	}
	// Hand the worktree back to the original content WITHOUT committing — the shape that broke the
	// worktree-only check: HEAD still holds the churn, so it must still be reported.
	mustWrite(t, repo, "f", "x") // newRepo's original content
	if churn, err := CommittedChurn(repo, original, []string{"f"}); err != nil || !churn {
		t.Fatalf("an uncommitted hand-edit must not hide committed churn still sitting in HEAD: churn=%v err=%v", churn, err)
	}
}

// TestWorktreeDirtyIsIndependentOfCommittedChurn is the other half of the same split: a dirty
// worktree and committed churn are different facts, checked separately so a caller can react to each.
func TestWorktreeDirtyIsIndependentOfCommittedChurn(t *testing.T) {
	repo := newRepo(t)
	if dirty, err := WorktreeDirty(repo, []string{"f"}); err != nil || dirty {
		t.Fatalf("an untouched worktree is not dirty: dirty=%v err=%v", dirty, err)
	}
	mustWrite(t, repo, "f", "hand-edited, uncommitted")
	if dirty, err := WorktreeDirty(repo, []string{"f"}); err != nil || !dirty {
		t.Fatalf("an uncommitted edit against HEAD is dirty: dirty=%v err=%v", dirty, err)
	}
}

// TestDroppingAPathTheBranchAddedRemovesIt is the defect that stranded a worker. Build output
// committed by accident is the commonest thing `git drop` is reached for, and the merge-base has no
// such path — `git checkout base -- <path>` answered "did not match any file(s) known to git",
// which travelled up as a hub fault and escalated the agent instead of dropping the file.
func TestDroppingAPathTheBranchAddedRemovesIt(t *testing.T) {
	repo := newRepo(t)
	base, _ := CurrentBranch(repo)
	if err := CreateBranch(repo, "feat", base); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, repo, "dist/index.html", "built")
	if err := CommitAll(repo, "commit the build output"); err != nil {
		t.Fatal(err)
	}
	target, err := MergeBase(repo, base, "feat")
	if err != nil {
		t.Fatal(err)
	}

	removed, err := RestoreFromRef(repo, target, []string{"dist/index.html"})
	if err != nil {
		t.Fatalf("dropping a path the branch added: %v", err)
	}
	if len(removed) != 1 || removed[0] != "dist/index.html" {
		t.Errorf("RestoreFromRef reported removed=%v; the caller says so in its reply", removed)
	}
	if _, err := os.Stat(filepath.Join(repo, "dist/index.html")); !os.IsNotExist(err) {
		t.Errorf("the dropped file is still on disk (stat err %v)", err)
	}
	if err := CommitAll(repo, "drop it"); err != nil {
		t.Fatal(err)
	}
	if churn, _ := CommittedChurn(repo, target, []string{"dist/index.html"}); churn {
		t.Error("after the drop the branch still carries committed churn for the path")
	}
}

// TestDroppingAMixOfAddedAndEditedPaths: one invocation carries both kinds, and the added one must
// not take the edited one down with it — the failing checkout took the whole list.
func TestDroppingAMixOfAddedAndEditedPaths(t *testing.T) {
	repo := newRepo(t)
	mustWrite(t, repo, "keep.go", "original")
	if err := CommitAll(repo, "base has keep.go"); err != nil {
		t.Fatal(err)
	}
	base, _ := CurrentBranch(repo)
	if err := CreateBranch(repo, "feat", base); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, repo, "keep.go", "edited")
	mustWrite(t, repo, "added.go", "new")
	if err := CommitAll(repo, "edit one, add one"); err != nil {
		t.Fatal(err)
	}
	target, err := MergeBase(repo, base, "feat")
	if err != nil {
		t.Fatal(err)
	}

	removed, err := RestoreFromRef(repo, target, []string{"keep.go", "added.go"})
	if err != nil {
		t.Fatalf("RestoreFromRef: %v", err)
	}
	if len(removed) != 1 || removed[0] != "added.go" {
		t.Errorf("removed = %v, want just added.go", removed)
	}
	body, err := os.ReadFile(filepath.Join(repo, "keep.go"))
	if err != nil {
		t.Fatalf("the edited path should have been restored, not removed: %v", err)
	}
	if string(body) != "original" {
		t.Errorf("keep.go = %q, want the base's content", body)
	}
}
