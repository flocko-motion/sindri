package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mustWrite writes rel (under repo), creating parent dirs.
func mustWrite(t *testing.T, repo, rel, body string) {
	t.Helper()
	p := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mustCommitOn checks branch out, writes rel, and commits it directly with git
// (bypassing CommitAll's .todos exclusion) — for building base history in tests.
func mustCommitOn(t *testing.T, repo, branch, rel, body string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", repo, "checkout", branch).CombinedOutput(); err != nil {
		t.Fatalf("checkout %s: %s", branch, out)
	}
	mustWrite(t, repo, rel, body)
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-qm", "c"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
}

// gitOut runs a git command in repo, failing the test on error.
func gitOut(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
	return string(out)
}

// TestRebaseStartLeavesConflictThenContinues covers the hub-assisted resolution
// loop: RebaseStart leaves a conflict in the worktree (does not abort), and after
// the worker resolves the file to base's version — making the branch's commit
// empty against base, the issue #27 case — RebaseContinue skips it and finishes.
func TestRebaseStartLeavesConflictThenContinues(t *testing.T) {
	repo := newRepo(t)
	def := strings.TrimSpace(gitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))

	gitOut(t, repo, "checkout", "-q", "-b", "feat")
	mustWrite(t, repo, "f", "feat\n")
	gitOut(t, repo, "commit", "-aqm", "feat change")

	gitOut(t, repo, "checkout", "-q", def)
	mustWrite(t, repo, "f", "base\n")
	gitOut(t, repo, "commit", "-aqm", "base change")

	conflicts, done, err := RebaseStart(repo, "feat", def)
	if err != nil {
		t.Fatalf("RebaseStart: %v", err)
	}
	if done {
		t.Fatal("expected a conflict, got done")
	}
	if len(conflicts) != 1 || conflicts[0] != "f" {
		t.Fatalf("conflicts = %v, want [f]", conflicts)
	}
	if !RebaseInProgress(repo) {
		t.Fatal("rebase should be left in progress for the worker to resolve")
	}

	// Worker resolves to base's version → the branch's commit is now empty against
	// base; RebaseContinue must --skip it and finish cleanly.
	mustWrite(t, repo, "f", "base\n")
	conflicts, done, err = RebaseContinue(repo)
	if err != nil {
		t.Fatalf("RebaseContinue: %v", err)
	}
	if !done || len(conflicts) > 0 {
		t.Fatalf("expected done with no conflicts, got done=%v conflicts=%v", done, conflicts)
	}
	if RebaseInProgress(repo) {
		t.Fatal("rebase should have finished")
	}
}

// TestAutostashConflictIsReportedNotCalledClean covers the trap that stranded agent dain: a rebase
// whose COMMITS apply cleanly, while --autostash re-applying the loose edits then clashes with the
// new base. git calls that a success (exit 0, "Successfully rebased") and owns no rebase
// afterwards, so reading only RebaseInProgress reported "aligned" and left an unmerged index that
// made every later checkout — the first thing RebaseStart does — fail for good.
func TestAutostashConflictIsReportedNotCalledClean(t *testing.T) {
	repo := newRepo(t)
	def := strings.TrimSpace(gitOut(t, repo, "rev-parse", "--abbrev-ref", "HEAD"))

	// The branch's own commit touches a different file, so the rebase itself cannot conflict.
	gitOut(t, repo, "checkout", "-q", "-b", "feat")
	mustWrite(t, repo, "other", "feat\n")
	gitOut(t, repo, "add", "-A")
	gitOut(t, repo, "commit", "-qm", "feat change")

	mustCommitOn(t, repo, def, "f", "base\n") // base moves f
	gitOut(t, repo, "checkout", "-q", "feat")
	mustWrite(t, repo, "f", "loose\n") // uncommitted, and it clashes with base's f

	conflicts, done, err := RebaseStart(repo, "feat", def)
	if err != nil {
		t.Fatalf("RebaseStart: %v", err)
	}
	if done {
		t.Fatal("an autostash that failed to re-apply must not be reported as a clean rebase")
	}
	if len(conflicts) != 1 || conflicts[0] != "f" {
		t.Fatalf("conflicts = %v, want [f]", conflicts)
	}
	// git owns nothing here — which is exactly why the plain rebase-in-progress check missed it.
	if RebaseInProgress(repo) {
		t.Fatal("no rebase should be in progress: the rebase itself finished")
	}
	if !StashConflict(repo) {
		t.Fatal("StashConflict must recognise the unmerged index the autostash left")
	}

	// Calling again before resolving must re-prompt, not stage the markers as a resolution.
	again, done, err := ResolveStashConflict(repo)
	if err != nil {
		t.Fatalf("ResolveStashConflict with markers still in place: %v", err)
	}
	if done || len(again) != 1 || again[0] != "f" {
		t.Fatalf("unresolved markers must come back as conflicts, got done=%v conflicts=%v", done, again)
	}

	mustWrite(t, repo, "f", "resolved\n") // the worker resolves
	conflicts, done, err = ResolveStashConflict(repo)
	if err != nil {
		t.Fatalf("ResolveStashConflict: %v", err)
	}
	if !done || len(conflicts) > 0 {
		t.Fatalf("expected done with no conflicts, got done=%v conflicts=%v", done, conflicts)
	}
	if StashConflict(repo) {
		t.Fatal("the unmerged index should be cleared")
	}
	// The blocker was the checkout, so prove that works again — and that nothing was lost.
	gitOut(t, repo, "checkout", "feat")
	if b, err := os.ReadFile(filepath.Join(repo, "f")); err != nil || string(b) != "resolved\n" {
		t.Fatalf("f = %q (err %v), want the worker's resolution", b, err)
	}
	if s := strings.TrimSpace(gitOut(t, repo, "stash", "list")); s != "" {
		t.Fatalf("the spent autostash entry should be dropped, got %q", s)
	}
}

// TestDeleteBranchRequiresDetach covers the scrap-PR teardown mechanic: git refuses to
// delete a branch that's checked out in a worktree (the agent's), so DetachHead must
// free it first — the exact ordering ScrapBranch relies on.
func TestDeleteBranchRequiresDetach(t *testing.T) {
	repo := newRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	if err := WorktreeAdd(repo, wt, "HEAD"); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if err := CreateBranch(wt, "feature", "HEAD"); err != nil { // branch now checked out in wt
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := DeleteBranch(repo, "feature"); err == nil {
		t.Fatal("DeleteBranch should fail while the branch is checked out in a worktree")
	}
	if err := DetachHead(wt); err != nil { // free the branch
		t.Fatalf("DetachHead: %v", err)
	}
	if err := DeleteBranch(repo, "feature"); err != nil {
		t.Fatalf("DeleteBranch after detach: %v", err)
	}
	if out := strings.TrimSpace(gitOut(t, repo, "branch", "--list", "feature")); out != "" {
		t.Fatalf("branch 'feature' should be gone, got %q", out)
	}
}

// newRepo creates a throwaway git repo with one commit and returns its root.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@t"},
		{"config", "user.name", "t"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-qm", "init"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	return dir
}

func TestRootAndCommits(t *testing.T) {
	repo := newRepo(t)
	got, err := Root(filepath.Join(repo))
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	// macOS/tmp symlinks can differ; compare resolved paths.
	want, _ := filepath.EvalSymlinks(repo)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != want {
		t.Fatalf("root: got %q want %q", gotResolved, want)
	}
	if ok, err := HasCommits(repo); err != nil || !ok {
		t.Fatalf("expected commits (ok=%v err=%v)", ok, err)
	}
}

func TestWorktreeAdd(t *testing.T) {
	repo := newRepo(t)
	wt := filepath.Join(repo, ".worktrees", "brokkr")
	if err := WorktreeAdd(repo, wt, "HEAD"); err != nil {
		t.Fatalf("worktree add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
		t.Fatalf("worktree .git missing: %v", err)
	}
	// Idempotent: second call on the existing worktree path must not error.
	if err := WorktreeAdd(repo, wt, "HEAD"); err != nil {
		t.Fatalf("worktree add (reuse): %v", err)
	}
}

func TestRebaseOntoCleanAndConflict(t *testing.T) {
	repo := newRepo(t)
	base, _ := CurrentBranch(repo)

	// A feature branch that edits a different file rebases cleanly onto an
	// advanced base.
	if err := CreateBranch(repo, "feat", base); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, repo, "feat.go", "feature")
	if err := CommitAll(repo, "feat"); err != nil {
		t.Fatal(err)
	}
	mustCommitOn(t, repo, base, "base.go", "base-moved")
	if err := RebaseOnto(repo, "feat", base); err != nil {
		t.Fatalf("clean rebase should succeed: %v", err)
	}

	// A branch that edits the SAME line as the advanced base conflicts; RebaseOnto
	// reports it and leaves the worktree clean (rebase aborted), not mid-rebase.
	if err := CreateBranch(repo, "clash", base); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, repo, "f", "branch-version")
	if err := CommitAll(repo, "clash"); err != nil {
		t.Fatal(err)
	}
	mustCommitOn(t, repo, base, "f", "base-version")
	if err := RebaseOnto(repo, "clash", base); err == nil {
		t.Fatal("a conflicting rebase must be reported as an error")
	}
	if changed, err := HasChanges(repo); err != nil || changed {
		t.Errorf("after a conflicting rebase the worktree must be clean (aborted), not mid-rebase (changed=%v err=%v)", changed, err)
	}
}

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

// TestMatchesRefTellsANoOpFromARealChange is what a caller needs before claiming a restore did
// anything: git's own checkout and commit both succeed identically on a no-op, so telling the two
// apart has to happen before either runs.
func TestMatchesRefTellsANoOpFromARealChange(t *testing.T) {
	repo := newRepo(t)
	original := strings.TrimSpace(gitOut(t, repo, "rev-parse", "HEAD"))
	if matches, err := MatchesRef(repo, original, []string{"f"}); err != nil || !matches {
		t.Fatalf("an untouched file should match the commit it came from: matches=%v err=%v", matches, err)
	}
	mustWrite(t, repo, "f", "changed")
	if err := CommitAll(repo, "diverge"); err != nil {
		t.Fatal(err)
	}
	if matches, err := MatchesRef(repo, original, []string{"f"}); err != nil || matches {
		t.Fatalf("a committed edit must be reported as differing from the original commit: matches=%v err=%v", matches, err)
	}
}
