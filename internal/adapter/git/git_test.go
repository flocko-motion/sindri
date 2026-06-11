package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

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
