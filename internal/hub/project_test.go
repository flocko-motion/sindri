package hub

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/flo-at/sindri/internal/api"
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
	if _, err := h.agents.NewAgent(tag, "eitri", "worker", ""); err != nil {
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

// TestGlobalProjectRegistersAtStartup: nothing ever names GlobalProject in a request the way a real
// repo's root does (there is no directory to lazily register from), so it must already be in the
// registry the moment the hub opens, with a path whose basename repoSlug reads back unchanged.
func TestGlobalProjectRegistersAtStartup(t *testing.T) {
	h := newHub(t)

	path, ok, err := h.store.ProjectPath(api.GlobalProject)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("GlobalProject should already be registered when the hub opens")
	}
	if got := filepath.Base(path); got != api.GlobalProject {
		t.Errorf("GlobalProject's registered path is %q, whose basename is %q, want %q", path, got, api.GlobalProject)
	}
}

// TestGlobalProjectCannotBeForgotten: forgetting it would tear down every reviewer in the pool for a
// project the hub re-registers on its very next restart anyway — a footgun with no matching benefit.
func TestGlobalProjectCannotBeForgotten(t *testing.T) {
	h := newHub(t)

	if err := h.projects.Forget(api.GlobalProject); err == nil {
		t.Fatal("forgetting GlobalProject should be refused")
	}
	if _, ok, _ := h.store.ProjectPath(api.GlobalProject); !ok {
		t.Error("a refused forget must leave the registry row in place")
	}
}
