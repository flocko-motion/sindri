package git

import (
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
