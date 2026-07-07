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

// HasCommits reports whether the repo has at least one commit. An unborn HEAD (no
// commits yet) is a legitimate false; a real git failure (not a repo, etc.) is
// returned rather than collapsed into "no commits".
func HasCommits(repo string) (bool, error) {
	err := exec.Command("git", "-C", repo, "rev-parse", "--verify", "-q", "HEAD").Run()
	if err == nil {
		return true, nil
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false, nil // `--verify -q` exits 1, quietly, when HEAD is unborn
	}
	return false, fmt.Errorf("git rev-parse HEAD in %s: %w", repo, err)
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

// CheckoutDetachedClean force-checks-out ref in detached HEAD (no branch claimed, so
// it can hold a branch checked out in another worktree) and removes untracked files,
// landing the worktree EXACTLY on ref's tip regardless of prior state. It's for the
// disposable review worktree: a reviewer only reads (and runs `sindri lint`, which
// builds and drops artifacts), so it produces nothing worth keeping — discarding is
// always safe, and forcing guarantees the reviewer sees the latest branch, never a
// stale checkout that a plain `git checkout` would refuse over local changes.
func CheckoutDetachedClean(dir, ref string) error {
	if out, err := exec.Command("git", "-C", dir, "checkout", "--detach", "--force", ref).CombinedOutput(); err != nil {
		return fmt.Errorf("checkout %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	if out, err := exec.Command("git", "-C", dir, "clean", "-fd").CombinedOutput(); err != nil {
		return fmt.Errorf("clean %s: %s: %w", dir, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// EnsureBranch puts dir's worktree on branch name, creating it from base if it
// doesn't exist yet — and preserving it (and any work on it) if it does. Used to
// give a planner a standing branch to draft openspec on.
func EnsureBranch(dir, name, base string) error {
	if cur, _ := CurrentBranch(dir); cur == name {
		return nil
	}
	if exec.Command("git", "-C", dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+name).Run() == nil {
		if out, err := exec.Command("git", "-C", dir, "checkout", name).CombinedOutput(); err != nil {
			return fmt.Errorf("checkout %s: %s: %w", name, strings.TrimSpace(string(out)), err)
		}
		return nil
	}
	if out, err := exec.Command("git", "-C", dir, "checkout", "-b", name, base).CombinedOutput(); err != nil {
		return fmt.Errorf("create branch %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Ahead reports whether dir's HEAD has any commit not in base (i.e. there's
// something to submit even with a clean worktree). A rev-list failure is returned,
// not collapsed into "not ahead" — that would silently skip a submit.
func Ahead(dir, base string) (bool, error) {
	out, err := exec.Command("git", "-C", dir, "rev-list", "--count", base+"..HEAD").Output()
	if err != nil {
		return false, fmt.Errorf("git rev-list %s..HEAD in %s: %w", base, dir, err)
	}
	n := strings.TrimSpace(string(out))
	return n != "" && n != "0", nil
}

// Rebase rebases dir's current branch onto onto. It aborts a rebase that hits
// conflicts (rather than leaving the worktree mid-rebase) and reports the error,
// so the caller can treat it as best-effort.
func Rebase(dir, onto string) error {
	if out, err := exec.Command("git", "-C", dir, "rebase", onto).CombinedOutput(); err != nil {
		_ = exec.Command("git", "-C", dir, "rebase", "--abort").Run()
		return fmt.Errorf("rebase onto %s: %s: %w", onto, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// HasChanges reports whether dir's worktree has uncommitted changes. A status
// failure is returned, not collapsed into "clean" — that would let CommitAll
// silently drop an agent's work.
func HasChanges(dir string) (bool, error) {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("git status in %s: %w", dir, err)
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

// CommitAll stages and commits everything in dir's worktree. A no-op (nil) when
// there is nothing to commit. It stages honestly (`git add -A`): an agent that
// reaches the task tracker only through the hub never touches `.todos/` in its
// worktree, so nothing churns it here — and if something ever does, it surfaces
// (a noisy diff, a loud merge failure) rather than being silently dropped.
func CommitAll(dir, msg string) error {
	changed, err := HasChanges(dir)
	if err != nil {
		return err
	}
	if !changed {
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

// RebaseOnto checks branch out in dir and rebases it onto onto. A conflict is
// reported (the rebase is aborted, leaving the worktree clean) so the caller can
// route it back to the owning worker. Used to bring a PR branch up to the current
// base before merging, so a merely-stale branch merges without human help.
func RebaseOnto(dir, branch, onto string) error {
	if out, err := exec.Command("git", "-C", dir, "checkout", branch).CombinedOutput(); err != nil {
		return fmt.Errorf("checkout %s: %s: %w", branch, strings.TrimSpace(string(out)), err)
	}
	return Rebase(dir, onto)
}

// RebaseInProgress reports whether dir has a rebase stopped mid-flight (conflict or
// an empty patch awaiting --skip). Resolves the real state path via git, since a
// worktree's .git is a file pointing elsewhere.
func RebaseInProgress(dir string) bool {
	for _, p := range []string{"rebase-merge", "rebase-apply"} {
		out, err := exec.Command("git", "-C", dir, "rev-parse", "--git-path", p).Output()
		if err != nil {
			continue
		}
		path := strings.TrimSpace(string(out))
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// gitEditless runs a git command in dir with editors forced non-interactive, so a
// `rebase --continue`/`--skip` that would otherwise open $EDITOR (for a commit
// message) never blocks the hub.
func gitEditless(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_EDITOR=true", "GIT_SEQUENCE_EDITOR=true")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// unmergedFiles lists the conflicted (unmerged) paths in dir's worktree.
func unmergedFiles(dir string) []string {
	return nameOnly(dir, "diff", "--name-only", "--diff-filter=U")
}

// RebaseStart checks branch out in dir and begins rebasing it onto onto WITHOUT
// aborting on conflict — the opposite of Rebase. It returns:
//   - done=true when the branch rebased cleanly (nothing to resolve);
//   - conflicts (the unmerged files) with done=false when it stopped on a conflict,
//     leaving the rebase in progress so a worker can resolve the file content and
//     the hub can continue it;
//   - a non-nil err only for a genuine failure (bad ref, etc.), with no rebase left
//     in progress.
func RebaseStart(dir, branch, onto string) (conflicts []string, done bool, err error) {
	if out, e := exec.Command("git", "-C", dir, "checkout", branch).CombinedOutput(); e != nil {
		return nil, false, fmt.Errorf("checkout %s: %s: %w", branch, strings.TrimSpace(string(out)), e)
	}
	// --autostash: an agent's incidental uncommitted edits (e.g. ticking task
	// checkboxes) are set aside and re-applied around the rebase, so they never block
	// a merge — the PR is the committed work, not the scratch in the worktree.
	out, e := gitEditless(dir, "rebase", "--autostash", onto)
	return settleRebase(dir, out, e)
}

// RebaseContinue stages the worker's conflict resolutions and advances a rebase
// that RebaseStart (or a prior RebaseContinue) left in progress. Return values
// match RebaseStart: done, or the next conflict set, or a hard error.
func RebaseContinue(dir string) (conflicts []string, done bool, err error) {
	if out, e := exec.Command("git", "-C", dir, "add", "-A").CombinedOutput(); e != nil {
		return nil, false, fmt.Errorf("git add: %s: %w", strings.TrimSpace(string(out)), e)
	}
	out, e := gitEditless(dir, "rebase", "--continue")
	return settleRebase(dir, out, e)
}

// settleRebase interprets the state after a rebase step (start/continue/skip) and
// drives past commits that became empty (already present in the base — e.g. a
// commit the base superseded): it --skips them so a redundant commit needs no
// worker action. It returns the next conflict set, done, or a hard error.
func settleRebase(dir, stepOut string, stepErr error) (conflicts []string, done bool, err error) {
	for {
		if !RebaseInProgress(dir) {
			if stepErr != nil { // rebase not in progress AND the step errored → genuine failure
				return nil, false, fmt.Errorf("rebase: %s: %w", strings.TrimSpace(stepOut), stepErr)
			}
			return nil, true, nil // rebase finished cleanly
		}
		if u := unmergedFiles(dir); len(u) > 0 {
			return u, false, nil // stopped on a real conflict — hand it to the worker
		}
		// In progress with no conflict = the current commit is empty against base;
		// skip it and re-evaluate.
		stepOut, stepErr = gitEditless(dir, "rebase", "--skip")
	}
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

// BlockingLocalChanges returns the tracked files in repo that a merge of branch
// (relative to base) would overwrite — git's "your local changes would be
// overwritten" set. Computed as the working-tree changes vs HEAD that also lie in
// the files the merge touches, so unrelated dirt (e.g. .todos churn the merge
// never touches) is excluded. Best-effort: returns nil on any git error.
func BlockingLocalChanges(repo, base, branch string) []string {
	touched := make(map[string]bool)
	for _, f := range nameOnly(repo, "diff", "--name-only", base+".."+branch) {
		touched[f] = true
	}
	var blocking []string
	for _, f := range nameOnly(repo, "diff", "--name-only", "HEAD") { // local changes vs HEAD
		if touched[f] {
			blocking = append(blocking, f)
		}
	}
	return blocking
}

// nameOnly runs a git name-only listing in repo and returns the non-empty paths.
func nameOnly(repo string, args ...string) []string {
	out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	return files
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
