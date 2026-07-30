package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolchainTooOldRecognisesTheRefusal(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string // fragments the advice must carry
	}{
		{
			name: "go.mod directive above the running toolchain",
			text: "load: err: exit status 1: stderr: go: go.mod requires go >= 1.26.5 (running go 1.26.4; GOTOOLCHAIN=local)",
			want: []string{"too old", "1.26.5", "1.26.4"},
		},
		{
			name: "a required module raises the floor",
			text: "go: example.com/dep@v1.2.3 requires go >= 1.27.0 (running go 1.26.5)",
			want: []string{"too old", "1.27.0", "1.26.5"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := toolchainTooOld(c.text)
			if got == "" {
				t.Fatalf("expected advice for %q", c.text)
			}
			for _, frag := range c.want {
				if !strings.Contains(got, frag) {
					t.Errorf("advice %q is missing %q", got, frag)
				}
			}
		})
	}
}

func TestToolchainTooOldIgnoresOtherFailures(t *testing.T) {
	for _, text := range []string{
		"",
		"load: err: exit status 1: stderr: go: cannot find main module",
		"packages contain errors",
		"internal/hub/state.go:12:5: undefined: Foo",
	} {
		if got := toolchainTooOld(text); got != "" {
			t.Errorf("toolchainTooOld(%q) = %q, want no advice", text, got)
		}
	}
}

// TestToolchainTooOldNamesTheInstaller pins the two voices apart: the advice names the installer
// where it exists, and stays generic where it doesn't — an agent must never be sent to a command
// its environment doesn't have.
func TestToolchainTooOldNamesTheInstaller(t *testing.T) {
	const text = "go: go.mod requires go >= 1.26.5 (running go 1.26.4; GOTOOLCHAIN=local)"

	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if got := toolchainTooOld(text); strings.Contains(got, UpgradeCommand) {
		t.Errorf("with no %s on PATH the advice must not name it, got: %q", UpgradeCommand, got)
	}

	script := filepath.Join(dir, UpgradeCommand)
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := toolchainTooOld(text); !strings.Contains(got, UpgradeCommand) {
		t.Errorf("with %s on PATH the advice must name it, got: %q", UpgradeCommand, got)
	}
}

// TestDeadcodeReportsToolchainTooOld drives the real thing: a module whose go directive no
// toolchain satisfies, so the go command refuses and the linter has to explain why.
func TestDeadcodeReportsToolchainTooOld(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"go.mod":  "module futuremod\n\ngo 1.99\n",
		"main.go": "package main\n\nfunc main() {}\n",
	})
	t.Setenv("GOTOOLCHAIN", "local") // no fetching our way out of it

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	var sb strings.Builder
	if _, err := Deadcode([]string{"./..."}, "", nil, nil, &sb); err == nil {
		t.Fatal("expected an error when the toolchain is older than go.mod requires")
	} else if !strings.Contains(err.Error(), "Go toolchain too old") {
		t.Errorf("the error must diagnose the toolchain, got: %v", err)
	}
}
