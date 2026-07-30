package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRebaseStepHealsAStrandedAutostashConflict covers the state agent dain was stuck in: the
// commits rebased, then --autostash clashed with the new base and git reported success anyway,
// leaving an unmerged index. RebaseStep must finish that instead of starting a fresh rebase, whose
// opening checkout git refuses while the index is unmerged — the loop no agent could break, since
// agents have no git of their own.
func TestRebaseStepHealsAStrandedAutostashConflict(t *testing.T) {
	repo := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
		return string(out)
	}
	write := func(rel, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init", "-q", "-b", "main")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	write("f", "base\n")
	write("other", "x\n")
	run("add", "-A")
	run("commit", "-qm", "init")

	// The branch's commit touches only "other", so the rebase itself applies cleanly.
	run("checkout", "-q", "-b", "feat")
	write("other", "feat\n")
	run("add", "-A")
	run("commit", "-qm", "feat")

	run("checkout", "-q", "main")
	write("f", "main\n")
	run("add", "-A")
	run("commit", "-qm", "main moves f")

	run("checkout", "-q", "feat")
	write("f", "loose\n") // uncommitted and clashing: the autostash will fail to re-apply

	conflicts, done, err := RebaseStep(repo, "feat", "main")
	if err != nil {
		t.Fatalf("RebaseStep: %v", err)
	}
	if done || len(conflicts) != 1 || conflicts[0] != "f" {
		t.Fatalf("expected the autostash clash reported as a conflict in [f], got done=%v conflicts=%v", done, conflicts)
	}

	// The worker resolves; the next step must heal rather than fail on a checkout.
	write("f", "resolved\n")
	conflicts, done, err = RebaseStep(repo, "feat", "main")
	if err != nil {
		t.Fatalf("RebaseStep after resolution: %v", err)
	}
	if !done || len(conflicts) > 0 {
		t.Fatalf("expected done, got done=%v conflicts=%v err=%v", done, conflicts, err)
	}
	// Aligned, resolution intact, and a further step is a no-op rather than an error.
	if got := strings.TrimSpace(run("log", "-1", "--format=%s", "main")); got != "main moves f" {
		t.Fatalf("base moved unexpectedly: %q", got)
	}
	b, err := os.ReadFile(filepath.Join(repo, "f"))
	if err != nil || string(b) != "resolved\n" {
		t.Fatalf("f = %q (err %v), want the worker's resolution", b, err)
	}
	if _, done, err = RebaseStep(repo, "feat", "main"); err != nil || !done {
		t.Fatalf("a healed worktree must rebase cleanly, got done=%v err=%v", done, err)
	}
}
