// package: adapter/git / commits
// type:    adapter (external tool: git)
// job:     name and compare the commits the hub reasons about — a branch's tip, a worktree's HEAD,
// an id resolved to a commit, whether one is reachable from another, and what lies between
// two of them.
// limits:  reads only; nothing here moves a ref or writes a commit (-> git.go).
package git

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// BranchTip is branch's current commit — what a moved reference is detected against.
func BranchTip(dir, branch string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--verify", "refs/heads/"+branch).Output()
	if err != nil {
		return "", fmt.Errorf("read tip of %s: %s", branch, gitError(err))
	}
	return strings.TrimSpace(string(out)), nil
}

// Head is dir's current commit. What a gate result is recorded against: a timestamp cannot say
// which tree was checked, and a sha can (-> hub/workflow.gateHead).
func Head(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("read HEAD in %s: %s", dir, gitError(err))
	}
	return strings.TrimSpace(string(out)), nil
}

// ResolveCommit expands rev to the full sha of the commit it names, or fails if it names none. A
// tag or branch resolves too — restricting WHAT may be named is the caller's business, not git's.
func ResolveCommit(dir, rev string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--verify", "--quiet", rev+"^{commit}").Output()
	if err != nil {
		return "", fmt.Errorf("%s names no commit in %s", rev, dir)
	}
	return strings.TrimSpace(string(out)), nil
}

// IsAncestor reports whether a is reachable from b. False for a branch that moved means its history
// was REWRITTEN (rebased, reset, force-moved), not merely advanced.
func IsAncestor(dir, a, b string) bool { return isAncestor(dir, a, b) }

// isAncestor reports whether commit a is reachable from b, so a branch can be fast-forwarded.
func isAncestor(dir, a, b string) bool {
	return exec.Command("git", "-C", dir, "merge-base", "--is-ancestor", a, b).Run() == nil
}

// CountRange counts every commit in from..to, merges included. LogRange hides merges because they
// read as noise in a list, which makes an empty LIST a bad test for "nothing arrived" — an advance
// made only of merge commits produces one. Callers deciding whether anything moved ask this; callers
// showing a human what moved ask LogRange.
func CountRange(dir, from, to string) (int, error) {
	out, err := exec.Command("git", "-C", dir, "rev-list", "--count", from+".."+to).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("git rev-list --count %s..%s: %s: %w", from, to, strings.TrimSpace(string(out)), err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("git rev-list --count %s..%s: unreadable count %q: %w", from, to, strings.TrimSpace(string(out)), err)
	}
	return n, nil
}

// LogRange lists "<short-sha> <subject>" for the commits in from..to (newest first), capped at
// max. Reused for both directions: a branch's own commits, and what its base has moved on by.
func LogRange(dir, from, to string, max int) ([]string, error) {
	args := []string{"-C", dir, "log", "--format=%h %s", "--no-merges"}
	if max > 0 {
		args = append(args, fmt.Sprintf("-%d", max))
	}
	out, err := exec.Command("git", append(args, from+".."+to)...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git log %s..%s: %s: %w", from, to, strings.TrimSpace(string(out)), err)
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}
