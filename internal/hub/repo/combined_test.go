package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/adapter/git"
)

// combineRepo builds a repo with a base branch and a feature branch off an older base, plus a
// worktree holding the feature branch — the shape a live PR has, since its author's tree owns it.
func combineRepo(t *testing.T) (root, agentWT string) {
	t.Helper()
	root = t.TempDir()
	if err := exec.Command("git", "init", "-q", "-b", "main", root).Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	run := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	write := func(dir, name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(root, "config", "user.email", "t@t")
	run(root, "config", "user.name", "t")
	write(root, "shared.txt", "one\n")
	run(root, "add", "-A")
	run(root, "commit", "-q", "-m", "base")

	// The PR's branch, in its author's own worktree — so nothing here may check it out again.
	agentWT = filepath.Join(root, ".worktrees", "bombur")
	run(root, "worktree", "add", "-q", "-b", "sd-1", agentWT, "HEAD")
	write(agentWT, "feature.txt", "from the PR\n")
	run(agentWT, "add", "-A")
	run(agentWT, "commit", "-q", "-m", "the PR's work")

	// base then moves on.
	write(root, "base-moved.txt", "later\n")
	run(root, "add", "-A")
	run(root, "commit", "-q", "-m", "base moves")
	return root, agentWT
}

// TestCombinedAppliesWithoutTouchingTheAuthorTree is the central guarantee: the check runs on a tree
// of its own. If it touched the author's worktree it would move the ground under an agent that may
// still be working, and a merge commit added to "check" the branch makes the real rebase harder —
// the very failure being looked for, caused by looking.
func TestCombinedAppliesWithoutTouchingTheAuthorTree(t *testing.T) {
	root, agentWT := combineRepo(t)
	tipBefore := revParse(t, agentWT, "HEAD")
	branchBefore := revParse(t, root, "sd-1")

	path, conflicts, err := MaterializeCombined(root, "sd-1", "main")
	if err != nil {
		t.Fatalf("MaterializeCombined: %v", err)
	}
	defer RemoveCombined(root)
	if len(conflicts) > 0 {
		t.Fatalf("disjoint changes should replay cleanly, got conflicts %v", conflicts)
	}
	// The combined tree holds both sides — this is what would actually land.
	for _, f := range []string{"feature.txt", "base-moved.txt"} {
		if _, err := os.Stat(filepath.Join(path, f)); err != nil {
			t.Errorf("the combined result is missing %s: %v", f, err)
		}
	}
	// And the author's tree and branch are exactly as they were.
	if got := revParse(t, agentWT, "HEAD"); got != tipBefore {
		t.Errorf("the author's worktree moved: %s → %s", tipBefore, got)
	}
	if got := revParse(t, root, "sd-1"); got != branchBefore {
		t.Errorf("the author's branch moved: %s → %s", branchBefore, got)
	}
}

// TestCombinedNamesTheConflictingPaths: a verdict with no evidence sends the author hunting, so the
// conflict has to come back with the paths in it.
func TestCombinedNamesTheConflictingPaths(t *testing.T) {
	root, agentWT := combineRepo(t)
	// Both sides edit the same line of the same file.
	writeCommit(t, agentWT, "shared.txt", "the PR's line\n", "PR edits shared")
	writeCommit(t, root, "shared.txt", "base's line\n", "base edits shared")

	_, conflicts, err := MaterializeCombined(root, "sd-1", "main")
	defer RemoveCombined(root)
	if err != nil {
		t.Fatalf("a conflict is a finding, not an error: %v", err)
	}
	if len(conflicts) == 0 {
		t.Fatal("competing edits to one line should conflict")
	}
	if !contains(conflicts, "shared.txt") {
		t.Errorf("the conflicting path should be named, got %v", conflicts)
	}
}

// TestCombinedSkipsACommitBaseAlreadyHas is why this replays rather than merging. A commit whose
// content is already in base conflicts on REPLAY while merging would call it clean — the gh-27 case.
// The rebase path skips it, so the check agrees with the merge the hub will perform.
func TestCombinedSkipsACommitBaseAlreadyHas(t *testing.T) {
	root, agentWT := combineRepo(t)
	// The same change on both sides, committed separately: identical content, different commits.
	writeCommit(t, agentWT, "both.txt", "same content\n", "PR adds both.txt")
	writeCommit(t, root, "both.txt", "same content\n", "base adds both.txt")

	_, conflicts, err := MaterializeCombined(root, "sd-1", "main")
	defer RemoveCombined(root)
	if err != nil {
		t.Fatalf("MaterializeCombined: %v", err)
	}
	if len(conflicts) > 0 {
		t.Errorf("a commit base already contains should be skipped, not reported: %v", conflicts)
	}
}

// TestRemoveCombinedLeavesNothingBehind: the throwaway is reserved by name and reused, so it has to
// clean up — both the worktree and the branch, or the next run cannot create the branch at all.
func TestRemoveCombinedLeavesNothingBehind(t *testing.T) {
	root, _ := combineRepo(t)
	path, _, err := MaterializeCombined(root, "sd-1", "main")
	if err != nil {
		t.Fatalf("MaterializeCombined: %v", err)
	}
	RemoveCombined(root)
	if _, err := os.Stat(path); err == nil {
		t.Error("the throwaway worktree survived its removal")
	}
	if git.BranchExists(root, combinedBranch()) {
		t.Error("the throwaway branch survived its removal")
	}
	// And a second run works, which is what the cleanup is for.
	if _, _, err := MaterializeCombined(root, "sd-1", "main"); err != nil {
		t.Errorf("a second check should be possible after cleanup: %v", err)
	}
	RemoveCombined(root)
}

// TestCombinedRecoversFromAStaleWorktree: a hub killed mid-check leaves the reserved tree behind.
// The next check must clear it rather than fail forever on a name already taken.
func TestCombinedRecoversFromAStaleWorktree(t *testing.T) {
	root, _ := combineRepo(t)
	if _, _, err := MaterializeCombined(root, "sd-1", "main"); err != nil {
		t.Fatalf("first: %v", err)
	}
	// No RemoveCombined: simulate the crash, then run again.
	if _, _, err := MaterializeCombined(root, "sd-1", "main"); err != nil {
		t.Errorf("a stale throwaway must not block the next check: %v", err)
	}
	RemoveCombined(root)
}

func revParse(t *testing.T, dir, ref string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", ref).Output()
	if err != nil {
		t.Fatalf("rev-parse %s in %s: %v", ref, dir, err)
	}
	return strings.TrimSpace(string(out))
}

func writeCommit(t *testing.T, dir, name, body, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", "-A").CombinedOutput(); err != nil {
		t.Fatalf("add: %s", out)
	}
	if out, err := exec.Command("git", "-C", dir, "commit", "-q", "-m", msg).CombinedOutput(); err != nil {
		t.Fatalf("commit: %s", out)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}
