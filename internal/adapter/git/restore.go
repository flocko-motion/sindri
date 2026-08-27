// package: adapter/git / restore
// type:    adapter (external tool: git)
// job:     put paths back to an earlier commit's content, and the two questions a caller needs
// answered before doing that: is there committed churn to drop, and is the worktree dirty.
// limits:  no podman, no task/PR logic; pure git.
package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// RestoreFromRef puts paths back to ref's content and stages that, reverting COMMITTED work.
// Returns the paths ref never carried, which it removed instead (-> absentFromRef).
func RestoreFromRef(dir, ref string, paths []string) ([]string, error) {
	present, absent, err := absentFromRef(dir, ref, paths)
	if err != nil {
		return nil, err
	}
	if len(present) > 0 {
		args := append([]string{"-C", dir, "checkout", ref, "--"}, present...)
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("restore from %s: %s: %w", ref, strings.TrimSpace(string(out)), err)
		}
	}
	if len(absent) > 0 {
		// -f: git rm refuses a path whose worktree copy differs from what was committed, and throwing
		// exactly that away is the verb's purpose.
		args := append([]string{"-C", dir, "rm", "-rqf", "--ignore-unmatch", "--"}, absent...)
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("remove %s (absent from %s): %s: %w",
				strings.Join(absent, " "), ref, strings.TrimSpace(string(out)), err)
		}
	}
	return absent, nil
}

// absentFromRef splits paths by whether ref carries them. Dropping one the branch ADDED means
// REMOVING it, which `git checkout ref -- <path>` answers by failing the whole invocation.
func absentFromRef(dir, ref string, paths []string) (present, absent []string, err error) {
	for _, p := range paths {
		// ls-tree over `cat-file -e`, which answers only for blobs: a path here may be a directory.
		out, err := exec.Command("git", "-C", dir, "ls-tree", "-r", "--name-only", ref, "--", p).Output()
		if err != nil {
			return nil, nil, fmt.Errorf("git ls-tree %s -- %s in %s: %s", ref, p, dir, gitError(err))
		}
		if strings.TrimSpace(string(out)) == "" {
			absent = append(absent, p)
			continue
		}
		present = append(present, p)
	}
	return present, absent, nil
}

// CommittedChurn reports whether ref's content differs from HEAD's for paths — the committed-
// history question BranchDiff's three-dot also asks, and the one that predicts whether restoring
// from ref would produce a commit. The worktree never enters into it: RestoreFromRef overwrites it
// with ref's content regardless of what was there before.
func CommittedChurn(dir, ref string, paths []string) (bool, error) {
	quiet, err := diffQuiet(dir, append([]string{ref, "HEAD", "--"}, paths...))
	if err != nil {
		return false, err
	}
	return !quiet, nil
}

// WorktreeDirty reports whether paths hold an uncommitted edit against HEAD — the other half of
// "is there anything here for a restore to do", since RestoreFromRef discards worktree edits on
// these paths whether or not they'd ever have been committed.
func WorktreeDirty(dir string, paths []string) (bool, error) {
	quiet, err := diffQuiet(dir, append([]string{"HEAD", "--"}, paths...))
	if err != nil {
		return false, err
	}
	return !quiet, nil
}

// diffQuiet runs `git diff --quiet <args>`, true when git found no difference. Exit 1 ("differs")
// becomes false rather than an error; any other non-zero exit is a genuine failure.
func diffQuiet(dir string, args []string) (bool, error) {
	err := exec.Command("git", append([]string{"-C", dir, "diff", "--quiet"}, args...)...).Run()
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git diff --quiet %v in %s: %w", args, dir, err)
}

// MergeBase returns where dir's branch and ref last agreed — the point a rebase treats as "mine
// started here", and the one target that lets a revert and a since-then diff agree on what "since"
// means even after ref has moved on independently.
func MergeBase(dir, ref, branch string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "merge-base", ref, branch).Output()
	if err != nil {
		return "", fmt.Errorf("git merge-base %s %s in %s: %s", ref, branch, dir, gitError(err))
	}
	return strings.TrimSpace(string(out)), nil
}
