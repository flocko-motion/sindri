// package: adapter/git / git
// type:    adapter (external tool: git)
// job:     wrap the git operations the hub needs — repo root, agent worktrees, branches,
// commits, rebase and merge. The only place git is invoked.
// limits:  no podman, no task/PR logic; pure git.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Root is the repository root containing dir — the MAIN worktree's root even from a linked one,
// because that is the path the hub registered.
func Root(dir string) (string, error) {
	top, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		// Relay git's own words: "exit status 128" hides the diagnosis, and a stale worktree
		// names the gitdir it can no longer find.
		return "", fmt.Errorf("not a git repo at %s: %s", dir, gitError(err))
	}
	toplevel := strings.TrimSpace(string(top))
	common, err := exec.Command("git", "-C", dir, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return toplevel, nil // older git without the flag: the checkout is the best answer
	}
	cd := strings.TrimSpace(string(common))
	if cd == "" {
		return toplevel, nil
	}
	// Relative to DIR, the directory git ran in — not to the toplevel. From `<repo>/.worktrees`
	// git says "../.git", which against the toplevel climbs out and names the repo's PARENT.
	if !filepath.IsAbs(cd) {
		base, aerr := filepath.Abs(dir)
		if aerr != nil {
			return toplevel, nil
		}
		cd = filepath.Join(base, cd)
	}
	// The common dir is the main checkout's `.git`, so its parent is that checkout — trusted
	// only when it looks like one, or an unusual layout resolves somewhere surprising.
	if filepath.Base(cd) != ".git" {
		return toplevel, nil
	}
	main := filepath.Dir(cd)
	if st, err := os.Stat(main); err != nil || !st.IsDir() {
		return toplevel, nil
	}
	return main, nil
}

// gitError renders a failed git invocation as its stderr, falling back to the exit status when
// it said nothing.
func gitError(err error) string {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if msg := strings.TrimSpace(string(ee.Stderr)); msg != "" {
			return msg
		}
	}
	return err.Error()
}

// WorktreeAdd creates a detached worktree at path on ref, pruning any stale registration first.
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

// WorktreeAddOnBranch adds a worktree on a NEW branch created at start. The new branch is the point:
// a worktree cannot check out a branch another worktree holds, and the branch under test is held by
// its author's tree.
func WorktreeAddOnBranch(repo, path, branch, start string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir worktree parent: %w", err)
	}
	out, err := exec.Command("git", "-C", repo, "worktree", "add", "-b", branch, path, start).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add -b %s: %s: %w", branch, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// RebaseHere replays dir's current branch onto onto, in place, reporting the conflicting paths or
// done. It skips commits base already contains, as the merge path does (-> settleRebase), since that
// is the question asked. Checks nothing out first, and aborts a conflict before returning.
func RebaseHere(dir, onto string) (conflicts []string, done bool, err error) {
	out, e := gitEditless(dir, "rebase", onto)
	conflicts, done, err = settleRebase(dir, out, e)
	if !done {
		_ = exec.Command("git", "-C", dir, "rebase", "--abort").Run()
	}
	return conflicts, done, err
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

// HasCommits reports whether the repo has a commit. An unborn HEAD is a legitimate false; a
// real git failure is returned rather than collapsed into one.
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
		// Relay git's own words and the dir: "exit status 128" alone named neither what failed
		// nor where, which is all a caller logging this had to go on.
		return "", fmt.Errorf("read current branch of %s: %s", dir, gitError(err))
	}
	b := strings.TrimSpace(string(out))
	if b == "" || b == "HEAD" {
		return "", fmt.Errorf("%s is on a detached HEAD", dir)
	}
	return b, nil
}

// BranchExists reports whether dir has a local branch of that name.
func BranchExists(dir, branch string) bool {
	return exec.Command("git", "-C", dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil
}

// AttachBranch reattaches a detached worktree to branch, reporting a rescue ref if it made one.
// Detaching frees a branch for deletion (-> DetachHead), so an agent could commit onto a HEAD no
// branch named. Nothing is discarded: the branch is created when missing, fast-forwarded when HEAD
// is ahead, and on divergence it stays put while the rescue ref names HEAD's commits.
func AttachBranch(dir, branch string) (rescue string, err error) {
	at, err := Head(dir)
	if err != nil {
		return "", fmt.Errorf("attach %s: %w", branch, err)
	}
	tip, tipErr := exec.Command("git", "-C", dir, "rev-parse", "--verify", "refs/heads/"+branch).Output()
	switch {
	case tipErr != nil: // no such branch — the deletion case; name HEAD and carry on
		return "", checkoutArgs(dir, branch, "-b", branch)
	case strings.TrimSpace(string(tip)) == at:
		return "", checkoutArgs(dir, branch, branch)
	case isAncestor(dir, strings.TrimSpace(string(tip)), at):
		return "", checkoutArgs(dir, branch, "-B", branch) // fast-forward; nothing to lose
	}
	// Diverged: moving the branch would drop its commits, attaching would drop HEAD's. Keep both.
	rescue = fmt.Sprintf("%s-detached-%s", branch, at[:7])
	if out, err := exec.Command("git", "-C", dir, "branch", "-f", rescue, at).CombinedOutput(); err != nil {
		return "", fmt.Errorf("rescue %s: %s", rescue, strings.TrimSpace(string(out)))
	}
	return rescue, checkoutArgs(dir, branch, branch)
}

// checkoutArgs runs a checkout, naming the branch in any failure.
func checkoutArgs(dir, branch string, args ...string) error {
	full := append([]string{"-C", dir, "checkout"}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		return fmt.Errorf("attach %s: %s", branch, strings.TrimSpace(string(out)))
	}
	return nil
}

// ResetBranchTo empties dir's checked-out branch back to ref — commits, tracked edits and untracked
// files go; the branch and dir's attachment to it survive. What a STANDING branch needs: deleting
// one would detach its worktree first, leaving the agent homeless.
func ResetBranchTo(dir, ref string) error {
	if out, err := exec.Command("git", "-C", dir, "reset", "--hard", ref).CombinedOutput(); err != nil {
		return fmt.Errorf("reset %s to %s: %s", dir, ref, strings.TrimSpace(string(out)))
	}
	// Uncommitted new files are part of the discarded work; ignored paths (td's store) are not.
	if out, err := exec.Command("git", "-C", dir, "clean", "-fd").CombinedOutput(); err != nil {
		return fmt.Errorf("clean %s: %s", dir, strings.TrimSpace(string(out)))
	}
	return nil
}

// ResetOntoKeepingWork resets dir's branch to ref, keeping uncommitted work (tracked and
// untracked) across the move by stashing then reapplying it rather than discarding it, as
// ResetBranchTo does. A clashing reapply leaves the shape a clashing `rebase --autostash` does,
// for the caller to route through StashConflict the same way.
func ResetOntoKeepingWork(dir, ref string) (conflicts []string, done bool, err error) {
	changed, err := HasChanges(dir)
	if err != nil {
		return nil, false, err
	}
	if changed {
		if out, e := exec.Command("git", "-C", dir, "stash", "push", "-u", "-m", "autostash").CombinedOutput(); e != nil {
			return nil, false, fmt.Errorf("stash uncommitted work in %s: %s: %w", dir, strings.TrimSpace(string(out)), e)
		}
	}
	if out, e := exec.Command("git", "-C", dir, "reset", "--hard", ref).CombinedOutput(); e != nil {
		return nil, false, fmt.Errorf("reset %s to %s: %s: %w", dir, ref, strings.TrimSpace(string(out)), e)
	}
	if !changed {
		return nil, true, nil
	}
	out, e := gitEditless(dir, "stash", "pop")
	if u := unmergedFiles(dir); len(u) > 0 {
		return u, false, nil // left for the worker, same shape a clashing autostash leaves
	}
	if e != nil {
		return nil, false, fmt.Errorf("reapply stashed work in %s: %s: %w", dir, strings.TrimSpace(out), e)
	}
	return nil, true, nil
}

// DetachHead detaches dir from its branch, freeing that branch for deletion elsewhere.
func DetachHead(dir string) error {
	if out, err := exec.Command("git", "-C", dir, "checkout", "--detach").CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout --detach in %s: %s: %w", dir, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// DeleteBranch force-deletes a local branch. git refuses while it is checked out anywhere, so
// detach that worktree first (-> DetachHead); unpushed work on it is intentionally discarded.
func DeleteBranch(repo, name string) error {
	if out, err := exec.Command("git", "-C", repo, "branch", "-D", name).CombinedOutput(); err != nil {
		return fmt.Errorf("git branch -D %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// CreateBranch creates and checks out a new branch from base in dir (an agent's
// worktree), discarding any prior checkout of that name.
func CreateBranch(dir, name, base string) error {
	if out, err := exec.Command("git", "-C", dir, "checkout", "-B", name, base).CombinedOutput(); err != nil {
		return fmt.Errorf("create branch %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// CheckoutDetachedClean lands dir exactly on ref's tip, detached and free of untracked files.
// For the disposable review worktree, where nothing is worth keeping and a stale read is worse.
func CheckoutDetachedClean(dir, ref string) error {
	if out, err := exec.Command("git", "-C", dir, "checkout", "--detach", "--force", ref).CombinedOutput(); err != nil {
		return fmt.Errorf("checkout %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
	}
	if out, err := exec.Command("git", "-C", dir, "clean", "-fd").CombinedOutput(); err != nil {
		return fmt.Errorf("clean %s: %s: %w", dir, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// EnsureBranch puts dir on name, creating it from base if absent and preserving any work on it
// if not — a planner's standing branch.
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

// Ahead reports whether dir has a commit not in base — something to submit even when the
// worktree is clean. A failure is returned, since "not ahead" would silently skip a submit.
func Ahead(dir, base string) (bool, error) {
	out, err := exec.Command("git", "-C", dir, "rev-list", "--count", base+"..HEAD").Output()
	if err != nil {
		return false, fmt.Errorf("git rev-list %s..HEAD in %s: %w", base, dir, err)
	}
	n := strings.TrimSpace(string(out))
	return n != "" && n != "0", nil
}

// Rebase rebases dir's branch onto onto, aborting on conflict rather than leaving it mid-rebase.
func Rebase(dir, onto string) error {
	if out, err := exec.Command("git", "-C", dir, "rebase", onto).CombinedOutput(); err != nil {
		_ = exec.Command("git", "-C", dir, "rebase", "--abort").Run()
		return fmt.Errorf("rebase onto %s: %s: %w", onto, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// HasChanges reports whether dir has uncommitted changes. A failure is returned, since "clean"
// would let CommitAll silently drop an agent's work.
func HasChanges(dir string) (bool, error) {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return false, fmt.Errorf("git status in %s: %w", dir, err)
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

// CommitAll stages everything in dir and commits it, or does nothing if there is nothing to
// commit. `git add -A` on purpose: anything unexpected should surface, not be filtered away.
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

// RebaseInProgress reports whether dir has a rebase stopped mid-flight (conflict, or an empty patch
// awaiting --skip). Asks git for the state path, since a worktree's .git is a file pointing away.
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

// gitEditless runs a git command in dir with editors forced non-interactive, so a `rebase
// --continue`/`--skip` needing a commit message never opens $EDITOR and blocks the hub.
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

// StashConflict reports unmerged entries with no rebase in progress — the state `rebase
// --autostash` leaves when the commits land but re-applying the stash clashes: git exits 0 saying
// "Successfully rebased", so only the index shows it, and it refuses every later checkout.
func StashConflict(dir string) bool {
	return !RebaseInProgress(dir) && len(unmergedFiles(dir)) > 0
}

// ResolveStashConflict accepts the worker's resolution of a StashConflict — staging is what clears
// the index, and the stash is then spent. Files still marked come back as conflicts, so a premature
// call re-prompts instead of staging "<<<<<<<". Returns as RebaseStart does.
func ResolveStashConflict(dir string) (conflicts []string, done bool, err error) {
	unmerged := unmergedFiles(dir)
	if marked := markedFiles(dir, unmerged); len(marked) > 0 {
		return marked, false, nil
	}
	if out, e := exec.Command("git", "-C", dir, "add", "-A").CombinedOutput(); e != nil {
		return nil, false, fmt.Errorf("stage stash resolution in %s: %s: %w", dir, strings.TrimSpace(string(out)), e)
	}
	if still := unmergedFiles(dir); len(still) > 0 {
		return nil, false, fmt.Errorf("index in %s still unmerged after staging: %s", dir, strings.Join(still, ", "))
	}
	dropSpentAutostash(dir)
	return nil, true, nil
}

// markedFiles returns those of files that still carry a conflict marker, so a resolution nobody
// finished is never staged as though it were done.
func markedFiles(dir string, files []string) []string {
	var out []string
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			out = append(out, f) // unreadable: unresolved is the safe reading, not "fine"
			continue
		}
		if bytes.HasPrefix(b, []byte(marker)) || bytes.Contains(b, []byte("\n"+marker)) {
			out = append(out, f)
		}
	}
	return out
}

// marker is git's conflict start line, the one the workers are told to look for.
const marker = "<<<<<<< "

// dropSpentAutostash removes the stash entry a clashing reapply leaves behind — spent once the
// resolution is staged, and left in place every later autostash piles another on. Two exact shapes
// only, never a stash anything else made: git's own `rebase --autostash` labels its entry bare
// "autostash"; ResetOntoKeepingWork's manual stash comes back "On <branch>: autostash".
func dropSpentAutostash(dir string) {
	out, err := exec.Command("git", "-C", dir, "stash", "list", "--format=%gd %gs").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hub: git: list stashes in %s: %s\n", dir, gitError(err))
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		ref, subject, ok := strings.Cut(strings.TrimSpace(line), " ")
		subject = strings.TrimSpace(subject)
		if !ok || (subject != "autostash" && !strings.HasSuffix(subject, ": autostash")) {
			continue
		}
		// Newest first, and dropping renumbers the rest — take this one and stop.
		if o, e := exec.Command("git", "-C", dir, "stash", "drop", ref).CombinedOutput(); e != nil {
			fmt.Fprintf(os.Stderr, "hub: git: drop spent autostash %s in %s: %s\n", ref, dir, strings.TrimSpace(string(o)))
		}
		return
	}
}

// RebaseStart rebases branch onto onto WITHOUT aborting on conflict: done when it landed
// clean, else the unmerged files with the rebase left in progress for a worker to resolve. An
// error means a genuine failure, with nothing in progress.
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

// settleRebase reads the state after a rebase step and skips commits the base already contains,
// so a redundant commit needs no worker action. Returns the next conflicts, done, or an error.
func settleRebase(dir, stepOut string, stepErr error) (conflicts []string, done bool, err error) {
	for {
		if !RebaseInProgress(dir) {
			if stepErr != nil { // rebase not in progress AND the step errored → genuine failure
				return nil, false, fmt.Errorf("rebase: %s: %w", strings.TrimSpace(stepOut), stepErr)
			}
			// The rebase landed, but --autostash re-applying the loose edits can conflict AFTER it,
			// and git reports that as success (-> StashConflict). Only the index shows it, so read
			// the index: calling this "done" told a worker it was aligned and left every later
			// checkout refusing on the dirty index, with no verb able to clear it.
			if u := unmergedFiles(dir); len(u) > 0 {
				return u, false, nil
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

// WorkingDiff returns dir's uncommitted changes (staged and unstaged) against HEAD, limited to
// paths when given. What an agent needs to see what it has actually touched but not yet committed.
func WorkingDiff(dir string, paths []string) (string, error) {
	return diffOut(dir, append([]string{"diff", "HEAD", "--"}, paths...)...)
}

// BranchDiff returns everything branch introduces over base (merge-base three-dot), limited to
// paths when given — the whole of an agent's change, committed work included.
func BranchDiff(dir, base, branch string, paths []string) (string, error) {
	return diffOut(dir, append([]string{"diff", base + "..." + branch, "--"}, paths...)...)
}

// diffOut runs a diff-shaped command, relaying git's own words on failure.
func diffOut(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", args[0], strings.TrimSpace(string(out)), err)
	}
	return string(out), nil
}

// ChangedNames lists the paths dir has uncommitted changes in, with git's status codes — the
// bounded answer to "what have I touched", where a full diff would be thousands of lines.
func ChangedNames(dir string) ([]string, error) {
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return nil, fmt.Errorf("git status in %s: %s", dir, gitError(err))
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

// BlockingLocalChanges returns the tracked files a merge of branch would overwrite: working-tree
// changes that also lie in what the merge touches, so unrelated dirt is excluded.
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

// gitEnglish runs a git command whose OUTPUT the hub matches on, with git's messages pinned to
// English — a translated "would be overwritten" made the match miss (-> repo.MergeBranch).
func gitEnglish(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	// Both: gettext consults LANGUAGE first, and ignores it only once the locale is already C.
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANGUAGE=C")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Merge squash-merges branch into base in repo, committing the squashed diff with msg and leaving
// base checked out. Squash, not a merge commit: base gets exactly one commit per PR however many
// the branch accumulated, and msg becomes the whole record of the change since the branch's own
// commit messages never reach base. Returns the combined output on conflict, in English since the
// caller reads it. msg's shape is the caller's business — this adapter just passes it to git.
func Merge(repo, base, branch, msg string) error {
	if out, err := gitEnglish(repo, "checkout", base); err != nil {
		return fmt.Errorf("checkout %s: %s: %w", base, strings.TrimSpace(out), err)
	}
	if out, err := gitEnglish(repo, "merge", "--squash", branch); err != nil {
		return fmt.Errorf("merge %s: %s: %w", branch, strings.TrimSpace(out), err)
	}
	// A branch already fully contained in base squashes to nothing staged — what --no-ff used to
	// report as "Already up to date" with no merge commit. `git commit` would refuse; skip it.
	staged, err := hasStagedChanges(repo)
	if err != nil {
		return err
	}
	if !staged {
		return nil
	}
	if out, err := gitEnglish(repo, "commit", "-m", msg); err != nil {
		return fmt.Errorf("commit squashed %s: %s: %w", branch, strings.TrimSpace(out), err)
	}
	return nil
}

// hasStagedChanges reports whether repo's index differs from HEAD. Answered by exit code, not
// nameOnly: that helper reads a failed git call as an empty list, which here would read a broken
// `git diff` as "nothing staged" and report a squash merged when nothing was ever committed.
func hasStagedChanges(repo string) (bool, error) {
	err := exec.Command("git", "-C", repo, "diff", "--cached", "--quiet").Run()
	if err == nil {
		return false, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return true, nil
	}
	return false, fmt.Errorf("check staged diff in %s: %w", repo, err)
}
