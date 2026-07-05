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
	if !HasCommits(repo) {
		t.Fatalf("expected commits")
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
	if HasChanges(repo) {
		t.Error("after a conflicting rebase the worktree must be clean (aborted), not mid-rebase")
	}
}
