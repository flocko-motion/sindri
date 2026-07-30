package workflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// refRepo builds a repo with branches main and side, leaving side checked out — so an inferred
// reference and a pinned one give different answers and the test can tell them apart.
func refRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "base")
	run("checkout", "-q", "-b", "side")
	return root
}

// writeRef pins reference in the repo's .sindri/config.yaml.
func writeRef(t *testing.T, root, branch string) {
	t.Helper()
	dir := filepath.Join(root, ".sindri")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("reference: "+branch+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestBaseBranchPinnedVsInferred is the hole this closes: unset, the reference is whatever the main
// checkout happens to be on, so switching branches to look at something silently redefined it for
// the whole fleet. Pinned, the checkout's state stops mattering.
func TestBaseBranchPinnedVsInferred(t *testing.T) {
	e := &Engine{}
	root := refRepo(t) // checked out on "side"

	got, err := e.baseBranch(root)
	if err != nil {
		t.Fatalf("baseBranch unset: %v", err)
	}
	if got != "side" {
		t.Errorf("unset should follow the checkout, got %q want \"side\"", got)
	}

	writeRef(t, root, "main")
	got, err = e.baseBranch(root)
	if err != nil {
		t.Fatalf("baseBranch pinned: %v", err)
	}
	if got != "main" {
		t.Errorf("pinned reference should win over the checkout, got %q want \"main\"", got)
	}

	// Still "main" after the checkout moves elsewhere — the point of pinning.
	if out, err := exec.Command("git", "-C", root, "checkout", "-q", "main").CombinedOutput(); err != nil {
		t.Fatalf("checkout main: %s", out)
	}
	if out, err := exec.Command("git", "-C", root, "checkout", "-q", "side").CombinedOutput(); err != nil {
		t.Fatalf("checkout side: %s", out)
	}
	if got, err = e.baseBranch(root); err != nil || got != "main" {
		t.Errorf("pinned reference must ignore the checkout, got %q (err %v)", got, err)
	}
}

// TestBaseBranchPinnedToAMissingBranchIsFatal: guessing here would silently measure every claim,
// submit and merge against a different branch, so a typo has to stop the operation and say so.
func TestBaseBranchPinnedToAMissingBranchIsFatal(t *testing.T) {
	e := &Engine{}
	root := refRepo(t)
	writeRef(t, root, "does-not-exist")
	got, err := e.baseBranch(root)
	if err == nil {
		t.Fatalf("a missing reference branch must be an error, got %q", got)
	}
	for _, want := range []string{"does-not-exist", "reference:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should name %q so it can be fixed: %v", want, err)
		}
	}
}
