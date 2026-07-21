// package: hub/repo / repo
// type:    logic (git/PR mechanics)
// job:     the git-backed operations the workflow orchestrates — materialize a PR
//          branch for inspection, run the lint gate against a worktree. Stateless:
//          each takes explicit paths/refs and returns a result or error; the workflow
//          resolves PR records and decides consequences.
// limits:  no store, no orchestration, no agent messaging. git primitives live in
//          adapter/git; this drives them for the PR lifecycle.
package repo

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/flo-at/sindri/internal/adapter/git"
)

// MaterializeReview checks out branch (detached) into the repo's reserved
// .worktrees/review workspace — fresh each time — and returns the path, so a human or
// reviewer can inspect a PR branch without disturbing any agent's own worktree.
func MaterializeReview(root, branch string) (string, error) {
	path := filepath.Join(root, ".worktrees", "review")
	_ = git.WorktreeRemove(root, path) // fresh checkout each time
	if err := git.WorktreeAdd(root, path, branch); err != nil {
		return "", err
	}
	return path, nil
}

// ScrapBranch removes a discarded PR's branch. It's normally checked out in the owning
// agent's worktree, and git won't delete a checked-out branch, so the worktree (when
// known and present) is detached from it first. The detach is best-effort — the branch
// may already be free — but the delete's outcome is returned so the caller can log a
// failure loudly rather than leave a lingering branch silently.
func ScrapBranch(root, worktree, branch string) error {
	if worktree != "" {
		if _, err := os.Stat(worktree); err == nil {
			_ = git.DetachHead(worktree)
		}
	}
	return git.DeleteBranch(root, branch)
}

// Lint runs the project's quality gate against a worktree by invoking `brokkr lint`
// there as a subprocess (so the concurrent hub never chdir's). Go modules only — a
// worktree with no go.mod has no gate and passes without needing the binary at all.
// resolveBin locates the brokkr toolbelt (the caller owns that lookup); a resolution
// failure surfaces as a loud lint failure rather than a silent pass.
func Lint(wt string, resolveBin func() (string, error)) (output string, ok bool) {
	if _, err := os.Stat(filepath.Join(wt, "go.mod")); err != nil {
		return "", true // no Go module — no lint gate applies
	}
	bin, err := resolveBin()
	if err != nil {
		return "lint: " + err.Error(), false
	}
	cmd := exec.Command(bin, "lint")
	cmd.Dir = wt
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}
