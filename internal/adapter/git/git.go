// package: adapter/git / git
// type:    adapter (external tool: git)
// job:     wrap the git operations the hub needs — locate the repo root, add a
//          detached worktree for an agent's workspace, read the current branch.
//          The only place git is invoked (fuller surface — rebase, merge —
//          lands in Phase 3).
// limits:  no podman, no task/PR logic; pure git.
package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Root returns the git repository root containing dir.
func Root(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not a git repo: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// WorktreeAdd creates a detached worktree at path pointing at ref (e.g. "HEAD"),
// creating the parent directory and pruning any stale registration first. It is
// safe to call when the worktree already exists for a fresh path.
func WorktreeAdd(repo, path, ref string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir worktree parent: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		// Path exists already; prune stale git metadata then reuse it.
		_ = exec.Command("git", "-C", repo, "worktree", "prune").Run()
		if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
			return nil // already a worktree
		}
	}
	out, err := exec.Command("git", "-C", repo, "worktree", "add", "--detach", path, ref).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// WorktreeRemove force-removes a worktree and prunes its registration. Safe to
// call when the worktree was never created (e.g. the agent never launched).
func WorktreeRemove(repo, path string) error {
	if _, err := os.Stat(path); err != nil {
		_ = exec.Command("git", "-C", repo, "worktree", "prune").Run()
		return nil // nothing to remove
	}
	out, err := exec.Command("git", "-C", repo, "worktree", "remove", "--force", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// HasCommits reports whether the repo has at least one commit.
func HasCommits(repo string) bool {
	return exec.Command("git", "-C", repo, "rev-parse", "HEAD").Run() == nil
}

// CurrentBranch returns the checked-out branch of dir, or an error in detached
// HEAD. Used to read the repo's base branch from the main checkout.
func CurrentBranch(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("current branch: %w", err)
	}
	b := strings.TrimSpace(string(out))
	if b == "" || b == "HEAD" {
		return "", fmt.Errorf("detached HEAD")
	}
	return b, nil
}

// CreateBranch creates and checks out a new branch from base in dir (an agent's
// worktree), discarding any prior checkout of that name.
func CreateBranch(dir, name, base string) error {
	if out, err := exec.Command("git", "-C", dir, "checkout", "-B", name, base).CombinedOutput(); err != nil {
		return fmt.Errorf("create branch %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// HasChanges reports whether dir's worktree has uncommitted changes.
func HasChanges(dir string) bool {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

// CommitAll stages and commits everything in dir's worktree. A no-op (nil) when
// there is nothing to commit.
func CommitAll(dir, msg string) error {
	if !HasChanges(dir) {
		return nil
	}
	if out, err := exec.Command("git", "-C", dir, "add", "-A").CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s: %w", strings.TrimSpace(string(out)), err)
	}
	if out, err := exec.Command("git", "-C", dir, "commit", "-m", msg).CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Diff returns the changes a branch introduces relative to base (the merge-base
// three-dot form), for review.
func Diff(repo, base, branch string) (string, error) {
	out, err := exec.Command("git", "-C", repo, "diff", base+"..."+branch).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// Merge merges branch into base in repo with a merge commit (no fast-forward),
// leaving base checked out. Returns the combined output on conflict.
func Merge(repo, base, branch string) error {
	if out, err := exec.Command("git", "-C", repo, "checkout", base).CombinedOutput(); err != nil {
		return fmt.Errorf("checkout %s: %s: %w", base, strings.TrimSpace(string(out)), err)
	}
	if out, err := exec.Command("git", "-C", repo, "merge", "--no-ff", "-m",
		"merge "+branch, branch).CombinedOutput(); err != nil {
		return fmt.Errorf("merge %s: %s: %w", branch, strings.TrimSpace(string(out)), err)
	}
	return nil
}
