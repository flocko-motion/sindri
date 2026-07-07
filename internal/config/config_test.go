package config

import (
	"os"
	"path/filepath"
	"testing"
)

// write a repo .sindri/config.yaml with the given body and return the repo root.
func repoWith(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("SINDRI_HOME", t.TempDir()) // isolate the global config layer
	dir := filepath.Join(root, ".sindri")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLoadDefaults(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SINDRI_HOME", t.TempDir())
	c, err := Load(root) // no config file at all
	if err != nil {
		t.Fatalf("absent config must not error: %v", err)
	}
	if c.Architecture != "ARCHITECTURE.md" || c.ArchitectureSet {
		t.Errorf("default architecture: got %q set=%v, want ARCHITECTURE.md unset", c.Architecture, c.ArchitectureSet)
	}
	if c.Containerfile != "" || c.ReviewPrompt != "" {
		t.Errorf("path defaults should be empty, got %+v", c)
	}
	if c.GitHub.Issues != nil {
		t.Error("unset github.issues should be nil (so explicit false is distinguishable)")
	}
	if !c.IssuesEnabled() {
		t.Error("github.issues defaults to ON (opt-out) when unset")
	}
}

// TestIssuesOptOut: the source is on by default; only an explicit `issues: false`
// disables it.
func TestIssuesOptOut(t *testing.T) {
	off := repoWith(t, "github:\n  issues: false\n")
	c, err := Load(off)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.IssuesEnabled() {
		t.Error("explicit github.issues: false must disable the source")
	}
}

func TestLoadValid(t *testing.T) {
	root := repoWith(t, "architecture: docs/ARCH.md\ngithub:\n  issues: true\n")
	if err := os.WriteFile(filepath.Join(root, "docs-ARCH-placeholder"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// architecture points at docs/ARCH.md — create it so the set-path check passes.
	_ = os.MkdirAll(filepath.Join(root, "docs"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "docs", "ARCH.md"), []byte("# arch"), 0o644)

	c, err := Load(root)
	if err != nil {
		t.Fatalf("valid config errored: %v", err)
	}
	if c.Architecture != "docs/ARCH.md" || !c.ArchitectureSet {
		t.Errorf("architecture: got %q set=%v", c.Architecture, c.ArchitectureSet)
	}
	if !c.IssuesEnabled() {
		t.Error("github.issues should be true")
	}
}

func TestLoadRejects(t *testing.T) {
	cases := []struct{ name, body string }{
		{"unknown key", "architektur: x\n"},
		{"wrong type", "github:\n  issues: nope\n"},
		{"absolute path", "review_prompt: /etc/passwd\n"},
		{"escapes repo", "containerfile: ../../evil\n"},
		{"missing set file", "architecture: does/not/exist.md\n"},
		{"malformed yaml", "architecture: [unterminated\n"},
	}
	for _, c := range cases {
		root := repoWith(t, c.body)
		if _, err := Load(root); err == nil {
			t.Errorf("%s: expected an error, got nil", c.name)
		}
	}
}

func TestRepoOverridesGlobal(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("SINDRI_HOME", home)
	// global sets github.issues true; repo leaves it unset but sets a review_prompt.
	_ = os.WriteFile(filepath.Join(home, "config.yaml"), []byte("github:\n  issues: true\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, ".sindri"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "review.txt"), []byte("review carefully"), 0o644)
	_ = os.WriteFile(filepath.Join(root, ".sindri", "config.yaml"), []byte("review_prompt: review.txt\n"), 0o644)

	c, err := Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !c.IssuesEnabled() { // inherited from the global layer
		t.Error("github.issues should be inherited from global")
	}
	if c.ReviewPrompt != "review.txt" { // set at the repo layer
		t.Errorf("review_prompt: got %q", c.ReviewPrompt)
	}
}
