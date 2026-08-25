package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// worktree builds a throwaway worktree; goMod adds a go.mod, which no longer changes what gates it.
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

// TestGateRefusesOnAFailingVerify is the point of the feature: work that fails the project's own
// checks must not reach a PR, and the reason has to come back with the refusal.
func TestGateRefusesOnAFailingVerify(t *testing.T) {
	wt := worktree(t, true)
	script(t, wt, "scripts/verify.sh", "echo 'FAIL: architecture test'\nexit 1\n")

	out, ok := Gate(t.Context(), wt, "scripts/verify.sh")
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

	out, ok := Gate(t.Context(), wt, "scripts/verify.sh")
	if !ok {
		t.Fatalf("a passing verify must let the submit through, got:\n%s", out)
	}
	if !strings.Contains(out, "all good") {
		t.Errorf("the gate's output should be kept, got:\n%s", out)
	}
}

// TestGateRunsWhateverTheLanguage: the declared gate decides, Go module or not.
func TestGateRunsWhateverTheLanguage(t *testing.T) {
	wt := worktree(t, false) // no go.mod: a TypeScript repo, say
	script(t, wt, "verify", "echo 'tsc failed'\nexit 1\n")

	out, ok := Gate(t.Context(), wt, "./verify")
	if ok {
		t.Fatalf("a declared gate must run on a non-Go tree, got:\n%s", out)
	}
	if !strings.Contains(out, "tsc failed") {
		t.Errorf("its output must come back, got:\n%s", out)
	}
}

// TestAnUndeclaredGateRefuses: the toolbelt used to stand in here, which left a project's real
// checks unrun — ranke-db's Go tests went ungated and its TypeScript rode on a linter blind to its
// types. An unanswered question refuses, whatever the tree contains.
func TestAnUndeclaredGateRefuses(t *testing.T) {
	for _, goMod := range []bool{true, false} {
		out, ok := Gate(t.Context(), worktree(t, goMod), "")
		if ok {
			t.Errorf("goMod=%v: a project with no declared gate passed; nothing checked it", goMod)
		}
		if !strings.Contains(out, "verify:") || !strings.Contains(out, ".sindri/config.yaml") {
			t.Errorf("goMod=%v: the refusal must say what to set and where, got:\n%s", goMod, out)
		}
	}
}

// TestGateReportsAMissingCommand: a declared gate that is not in the worktree is a configuration
// fault, and saying so beats a bare non-zero exit.
func TestGateReportsAMissingCommand(t *testing.T) {
	wt := worktree(t, true)
	out, ok := Gate(t.Context(), wt, "scripts/not-there.sh")
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
