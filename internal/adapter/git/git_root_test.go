package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestRootResolvesWorktreeToMainRepo: a command run inside a linked worktree must address the
// repo the hub registered, not the checkout it happens to stand in. `--show-toplevel` answers
// the latter, which is a path the hub has never seen.
func TestRootResolvesWorktreeToMainRepo(t *testing.T) {
	root := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	main := filepath.Join(root, "repo")
	if err := exec.Command("git", "init", "-q", "-b", "main", main).Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	run(main, "config", "user.email", "t@t")
	run(main, "config", "user.name", "t")
	run(main, "commit", "-q", "--allow-empty", "-m", "init")
	wt := filepath.Join(main, ".worktrees", "dvalin")
	run(main, "worktree", "add", "-q", wt, "HEAD")

	// Every vantage point inside the repo must name the same project: the main checkout, a
	// subdirectory of it, the directory that HOLDS the worktrees, a linked worktree, and a
	// subdirectory of that one.
	//
	// `<repo>/.worktrees` is the case that regressed. git answers `--git-common-dir` relative to
	// the directory it ran in, so from there it says "../.git"; resolved against the toplevel
	// instead, that climbed out of the repo and named its parent — an unregistered path, so the
	// TUI opened with no repo and an empty backlog.
	sub := filepath.Join(wt, "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(main, "internal", "pkg")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{main, inner, filepath.Join(main, ".worktrees"), wt, sub} {
		got, err := Root(dir)
		if err != nil {
			t.Fatalf("Root(%s): %v", dir, err)
		}
		want, _ := filepath.EvalSymlinks(main)
		gotEval, _ := filepath.EvalSymlinks(got)
		if gotEval != want {
			t.Errorf("Root(%s) = %s, want the main repo %s", dir, got, main)
		}
	}
}
