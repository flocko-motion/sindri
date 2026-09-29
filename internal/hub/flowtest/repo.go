// package: hub/flowtest / repo
// type:    assembly (a git repo a subject's tests can act on)
// job:     lay down the working tree every gate, submit and merge test needs — a repo with a base
// branch, an agent's worktree on its own branch, and the small writes that move either side.
// limits:  scaffolding. What a subject DOES with the tree is its own package's to assert.
package flowtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/adapter/git"
)

// WorkRepo builds a throwaway git repo with one commit on the default branch and a
// feature-branch worktree at .worktrees/<agent> checked out from it — the shape the
// hub lays down for a worker. Returns the repo root and the base branch name.
func WorkRepo(t *testing.T, agent, branch string) (root, base string) {
	t.Helper()
	root = t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run(root, "init", "-q")
	run(root, "config", "user.email", "t@t")
	run(root, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(root, "seed"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(root, "add", ".")
	run(root, "commit", "-qm", "init")
	// Named rather than inherited from the machine's init.defaultBranch: a test that hardcodes its
	// base passed locally on "main" and failed CI on "master", which is a fixture reporting the
	// runner instead of the code. Renamed after the first commit, so it needs no particular git.
	run(root, "branch", "-M", "main")
	base, err := git.CurrentBranch(root)
	if err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(root, ".worktrees", agent)
	if err := git.WorktreeAdd(root, wt, "HEAD"); err != nil {
		t.Fatalf("worktree add: %v", err)
	}
	if err := git.CreateBranch(wt, branch, base); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	return root, base
}

// WriteFile writes a file, creating what it needs.
func WriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// WriteExec writes a runnable script — a project's own gate command, for a test that needs the
// gate to answer one way or the other.
func WriteExec(t *testing.T, path, body string) {
	t.Helper()
	WriteFile(t, path, body)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// GitIn runs a git command that must succeed.
func GitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
}

// CommitIn writes and commits a file, so a test can move either side of a base.
func CommitIn(t *testing.T, dir, name, body, msg string) {
	t.Helper()
	WriteFile(t, filepath.Join(dir, name), body)
	GitIn(t, dir, "add", name)
	GitIn(t, dir, "commit", "-qm", msg)
}

// PinReference writes the repo's .sindri/config.yaml so the base branch is DECLARED rather than
// inferred from whatever the main checkout is on — which is the whole point of pinning it.
func PinReference(t *testing.T, root, branch string) {
	t.Helper()
	WriteFile(t, filepath.Join(root, ".sindri", "config.yaml"), "reference: "+branch+"\n")
}
