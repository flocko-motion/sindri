package hub

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRepoInitScaffoldsAndRegisters: init registers the repo and writes a
// .sindri/config.yaml template, and is idempotent + non-destructive (a second init
// leaves an existing, user-edited config untouched).
func TestRepoInitScaffoldsAndRegisters(t *testing.T) {
	h := newHub(t)
	root := t.TempDir()

	if _, err := h.projects.Init(root); err != nil {
		t.Fatalf("init: %v", err)
	}
	tag := RepoTag(root)
	if _, ok, _ := h.store.ProjectPath(tag); !ok {
		t.Fatal("init should register the repo")
	}
	cfgPath := filepath.Join(root, ".sindri", "config.yaml")
	if _, err := os.ReadFile(cfgPath); err != nil {
		t.Fatalf("init should scaffold %s: %v", cfgPath, err)
	}

	// A user edits the config; a second init must not clobber it.
	if err := os.WriteFile(cfgPath, []byte("github:\n  issues: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.projects.Init(root); err != nil {
		t.Fatalf("second init: %v", err)
	}
	got, _ := os.ReadFile(cfgPath)
	if string(got) != "github:\n  issues: false\n" {
		t.Errorf("init clobbered an existing config: %q", got)
	}
}

// TestRepoForgetDeletesAgentsKeepsRepo: forget tears down the repo's agents and
// drops the registry row, but leaves the repo's files (here, the scaffolded config)
// on disk — a soft forget for records, a hard teardown for agents.
func TestRepoForgetDeletesAgentsKeepsRepo(t *testing.T) {
	h := newHub(t)
	root := t.TempDir()
	if _, err := h.projects.Init(root); err != nil {
		t.Fatal(err)
	}
	tag := RepoTag(root)
	if _, err := h.NewAgent(tag, "eitri", "worker", ""); err != nil {
		t.Fatal(err)
	}

	if err := h.projects.Forget(tag); err != nil {
		t.Fatalf("forget: %v", err)
	}
	// Agents are deleted.
	if roster, _ := h.store.For(tag).Roster(); len(roster) != 0 {
		t.Fatalf("forget should delete the repo's agents, %d remain", len(roster))
	}
	// Registry row is gone.
	if _, ok, _ := h.store.ProjectPath(tag); ok {
		t.Error("forget should drop the registry row")
	}
	// The repo's files (scaffolded config) survive — forget is not delete.
	if _, err := os.ReadFile(filepath.Join(root, ".sindri", "config.yaml")); err != nil {
		t.Errorf("forget must not delete the repo's .sindri/config.yaml: %v", err)
	}
}
