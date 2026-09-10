package git

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// attachRepo builds a repo with one commit and returns its path.
func attachRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := exec.Command("git", "init", "-q", "-b", "main", dir).Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	for _, args := range [][]string{
		{"config", "user.email", "t@t"}, {"config", "user.name", "t"},
		{"commit", "-q", "--allow-empty", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	return dir
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
	return strings.TrimSpace(string(out))
}

// TestAttachBranchRecreatesADeletedBranch is the reported failure: freeing a branch for deletion
// detaches the worktree, the agent keeps committing, and the branch is gone — so a rebase had
// nothing to stand on. Attaching must name HEAD, keeping those commits.
func TestAttachBranchRecreatesADeletedBranch(t *testing.T) {
	dir := attachRepo(t)
	gitRun(t, dir, "checkout", "-q", "-b", "plan-galar")
	gitRun(t, dir, "commit", "-q", "--allow-empty", "-m", "planner work")
	gitRun(t, dir, "checkout", "-q", "--detach")
	gitRun(t, dir, "commit", "-q", "--allow-empty", "-m", "committed while detached")
	want := gitRun(t, dir, "rev-parse", "HEAD")
	gitRun(t, dir, "branch", "-q", "-D", "plan-galar") // the deletion that stranded it

	rescue, err := AttachBranch(dir, "plan-galar")
	if err != nil {
		t.Fatal(err)
	}
	if rescue != "" {
		t.Errorf("nothing had to be rescued, got %q", rescue)
	}
	if got, _ := CurrentBranch(dir); got != "plan-galar" {
		t.Errorf("worktree should be on plan-galar, got %q", got)
	}
	if got := gitRun(t, dir, "rev-parse", "HEAD"); got != want {
		t.Errorf("the detached commit must survive: HEAD %s, want %s", got, want)
	}
}

// TestAttachBranchFastForwards: when the branch still exists behind HEAD, moving it forward loses
// nothing, so no rescue ref should clutter the repo.
func TestAttachBranchFastForwards(t *testing.T) {
	dir := attachRepo(t)
	gitRun(t, dir, "checkout", "-q", "-b", "td-1")
	gitRun(t, dir, "checkout", "-q", "--detach")
	gitRun(t, dir, "commit", "-q", "--allow-empty", "-m", "ahead of the branch")
	want := gitRun(t, dir, "rev-parse", "HEAD")

	rescue, err := AttachBranch(dir, "td-1")
	if err != nil {
		t.Fatal(err)
	}
	if rescue != "" {
		t.Errorf("a fast-forward needs no rescue, got %q", rescue)
	}
	if got := gitRun(t, dir, "rev-parse", "td-1"); got != want {
		t.Errorf("td-1 should have moved to HEAD, got %s want %s", got, want)
	}
}

// TestAttachBranchRescuesDivergence: when the branch holds commits HEAD does not, neither side may
// be dropped — the branch stays put and HEAD's work is named, so nothing is lost silently.
func TestAttachBranchRescuesDivergence(t *testing.T) {
	dir := attachRepo(t)
	gitRun(t, dir, "checkout", "-q", "-b", "td-2")
	gitRun(t, dir, "commit", "-q", "--allow-empty", "-m", "on the branch")
	branchTip := gitRun(t, dir, "rev-parse", "HEAD")
	gitRun(t, dir, "checkout", "-q", "--detach", "HEAD~1")
	gitRun(t, dir, "commit", "-q", "--allow-empty", "-m", "diverged work")
	detached := gitRun(t, dir, "rev-parse", "HEAD")

	rescue, err := AttachBranch(dir, "td-2")
	if err != nil {
		t.Fatal(err)
	}
	if rescue == "" {
		t.Fatal("divergence must produce a rescue ref")
	}
	if got := gitRun(t, dir, "rev-parse", rescue); got != detached {
		t.Errorf("rescue ref should hold the detached commit, got %s want %s", got, detached)
	}
	if got := gitRun(t, dir, "rev-parse", "td-2"); got != branchTip {
		t.Errorf("the branch must not move, got %s want %s", got, branchTip)
	}
	if got, _ := CurrentBranch(dir); got != "td-2" {
		t.Errorf("worktree should be on td-2, got %q", got)
	}
}

// TestAttachBranchInWorktree: the real caller is a linked worktree, not the main checkout.
func TestAttachBranchInWorktree(t *testing.T) {
	main := attachRepo(t)
	wt := filepath.Join(main, ".worktrees", "galar")
	gitRun(t, main, "worktree", "add", "-q", "--detach", wt, "HEAD")
	gitRun(t, wt, "commit", "-q", "--allow-empty", "-m", "worktree work")
	want := gitRun(t, wt, "rev-parse", "HEAD")

	if _, err := AttachBranch(wt, "plan-galar"); err != nil {
		t.Fatal(err)
	}
	if got, _ := CurrentBranch(wt); got != "plan-galar" {
		t.Errorf("worktree should be on plan-galar, got %q", got)
	}
	if got := gitRun(t, wt, "rev-parse", "HEAD"); got != want {
		t.Errorf("commit lost: HEAD %s, want %s", got, want)
	}
}
