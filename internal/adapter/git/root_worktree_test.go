package git

import (
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

	// Both the main checkout and a nested subdirectory of the worktree resolve to the repo.
	sub := filepath.Join(wt, "src")
	if err := exec.Command("mkdir", "-p", sub).Run(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{main, wt, sub} {
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
