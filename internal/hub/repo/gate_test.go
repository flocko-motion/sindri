package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// worktree builds a throwaway worktree; goMod adds a go.mod so the built-in lint applies.
func worktree(t *testing.T, goMod bool) string {
	t.Helper()
	dir := t.TempDir()
	if goMod {
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.26\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// script writes an executable gate at rel inside dir.
func script(t *testing.T, dir, rel, body string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// noBrokkr stands in for a resolvable lint binary that passes, so a test can isolate the declared
// gate from the built-in one.
func passingLint(t *testing.T, dir string) func() (string, error) {
	t.Helper()
	script(t, dir, "fake-lint", "exit 0\n")
	return func() (string, error) { return filepath.Join(dir, "fake-lint"), nil }
}

// TestGateRefusesOnAFailingVerify is the point of the feature: work that fails the project's own
// checks must not reach a PR, and the reason has to come back with the refusal.
func TestGateRefusesOnAFailingVerify(t *testing.T) {
	wt := worktree(t, true)
	script(t, wt, "scripts/verify.sh", "echo 'FAIL: architecture test'\nexit 1\n")

	out, ok := Gate(wt, passingLint(t, wt), "scripts/verify.sh")
	if ok {
		t.Fatal("a failing verify must refuse the submit")
	}
	if !strings.Contains(out, "FAIL: architecture test") {
		t.Errorf("the gate's own output must come back, got:\n%s", out)
	}
	if !strings.Contains(out, "scripts/verify.sh") {
		t.Errorf("the refusal should name what failed, got:\n%s", out)
	}
}

// TestGatePassesWhenVerifyPasses: the whole gate is only as useful as its green path.
func TestGatePassesWhenVerifyPasses(t *testing.T) {
	wt := worktree(t, true)
	script(t, wt, "scripts/verify.sh", "echo all good\nexit 0\n")

	out, ok := Gate(wt, passingLint(t, wt), "scripts/verify.sh")
	if !ok {
		t.Fatalf("a passing verify must let the submit through, got:\n%s", out)
	}
	if !strings.Contains(out, "all good") {
		t.Errorf("the gate's output should be kept, got:\n%s", out)
	}
}

// TestGateRunsWhateverTheLanguage is the silent-pass fix: a project with no go.mod had NO gate at
// all. Once it declares one, that gate decides.
func TestGateRunsWhateverTheLanguage(t *testing.T) {
	wt := worktree(t, false) // no go.mod: a TypeScript repo, say
	script(t, wt, "verify", "echo 'tsc failed'\nexit 1\n")

	out, ok := Gate(wt, passingLint(t, wt), "verify")
	if ok {
		t.Fatalf("a declared gate must run on a non-Go tree, got:\n%s", out)
	}
	if !strings.Contains(out, "tsc failed") {
		t.Errorf("its output must come back, got:\n%s", out)
	}
}

// TestGateUnchangedWithoutAVerifyKey: existing repos must submit exactly as before — including the
// pass for a tree with no go.mod, which is current behaviour and not this change's to alter.
func TestGateUnchangedWithoutAVerifyKey(t *testing.T) {
	if out, ok := Gate(worktree(t, false), passingLint(t, t.TempDir()), ""); !ok || out != "" {
		t.Errorf("a non-Go tree with no declared gate should pass silently, got ok=%v out=%q", ok, out)
	}

	wt := worktree(t, true)
	script(t, wt, "failing-lint", "echo 'lint: bad'\nexit 1\n")
	resolve := func() (string, error) { return filepath.Join(wt, "failing-lint"), nil }
	if out, ok := Gate(wt, resolve, ""); ok {
		t.Errorf("the built-in lint must still refuse, got:\n%s", out)
	}
}

// TestGateRefusesBeforeRunningVerifyWhenLintFails: the built-in runs first, so a lint failure is
// reported without spending minutes on a build the work has already failed.
func TestGateRefusesBeforeRunningVerifyWhenLintFails(t *testing.T) {
	wt := worktree(t, true)
	script(t, wt, "failing-lint", "echo 'lint: bad'\nexit 1\n")
	script(t, wt, "verify", "echo VERIFY-RAN\nexit 0\n")

	out, ok := Gate(wt, func() (string, error) { return filepath.Join(wt, "failing-lint"), nil }, "verify")
	if ok {
		t.Fatal("a failing built-in lint must refuse")
	}
	if strings.Contains(out, "VERIFY-RAN") {
		t.Errorf("the declared gate should not run once the built-in has already refused:\n%s", out)
	}
}

// TestGateReportsAMissingCommand: a declared gate that is not in the worktree is a configuration
// fault, and saying so beats a bare non-zero exit.
func TestGateReportsAMissingCommand(t *testing.T) {
	wt := worktree(t, true)
	out, ok := Gate(wt, passingLint(t, wt), "scripts/not-there.sh")
	if ok {
		t.Fatal("a missing gate command must refuse rather than pass")
	}
	if !strings.Contains(out, "not found") || !strings.Contains(out, "scripts/not-there.sh") {
		t.Errorf("the refusal should name the missing path, got:\n%s", out)
	}
}

// TestCapLinesNamesWhatItCut: a gate can print thousands of lines, and silent truncation would read
// as the whole story. The TAIL is kept — a build's failure is at the end, not the start.
func TestCapLinesNamesWhatItCut(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString("line\n")
	}
	b.WriteString("THE FAILURE\n")

	got := capLines(b.String(), 10)
	if !strings.Contains(got, "THE FAILURE") {
		t.Error("the end of the output — where a failure is — must survive capping")
	}
	if !strings.Contains(got, "truncated") || !strings.Contains(got, "101") {
		t.Errorf("capping must say how much was cut, got:\n%s", got)
	}
	if n := strings.Count(got, "\n"); n > 12 {
		t.Errorf("capped output should be about the cap, got %d lines", n)
	}
	// Short output passes through untouched, and empty stays empty.
	if got := capLines("one\ntwo\n", 10); got != "one\ntwo\n" {
		t.Errorf("short output must be unchanged, got %q", got)
	}
	if got := capLines("", 10); got != "" {
		t.Errorf("empty output must stay empty, got %q", got)
	}
}
