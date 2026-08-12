package claude

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/adapter/agent"
)

// hostHome points ~/.claude at a temp dir and returns it, so a test can plant (or omit) the user's
// own files without touching the real home.
func hostHome(t *testing.T) string {
	t.Helper()
	host := t.TempDir()
	t.Setenv("HOME", host)
	dir := filepath.Join(host, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// prepare runs PrepareHome into a fresh pod home and returns that directory.
func prepare(t *testing.T, dir string) string {
	t.Helper()
	if _, err := (Claude{}).PrepareHome(agent.HomeSpec{Dir: dir, SystemPrompt: "prompt", Out: io.Discard}); err != nil {
		t.Fatalf("PrepareHome: %v", err)
	}
	return dir
}

// TestKeybindingsAreStaged: the point of the feature — the user's key map reaches the pod.
func TestKeybindingsAreStaged(t *testing.T) {
	hostDir := hostHome(t)
	const body = `{"submit":"ctrl+enter"}`
	if err := os.WriteFile(filepath.Join(hostDir, "keybindings.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	pod := prepare(t, filepath.Join(t.TempDir(), "home"))
	got, err := os.ReadFile(filepath.Join(pod, "keybindings.json"))
	if err != nil {
		t.Fatalf("keybindings were not staged: %v", err)
	}
	if string(got) != body {
		t.Errorf("staged %q, want %q", got, body)
	}
}

// TestNoKeybindingsStagesNothing: most users have none, and inventing an empty file could override
// Claude's own defaults with nothing.
func TestNoKeybindingsStagesNothing(t *testing.T) {
	hostHome(t)
	pod := prepare(t, filepath.Join(t.TempDir(), "home"))
	if _, err := os.Stat(filepath.Join(pod, "keybindings.json")); !os.IsNotExist(err) {
		t.Errorf("expected no keybindings.json, stat err = %v", err)
	}
}

// TestStaleKeybindingsAreRemoved is the case a plain copy would get wrong: this home persists per
// agent across launches, so a staged copy has to go when the user deletes theirs — otherwise the
// pod keeps answering to a key map that no longer exists anywhere on the host.
func TestStaleKeybindingsAreRemoved(t *testing.T) {
	hostDir := hostHome(t)
	kb := filepath.Join(hostDir, "keybindings.json")
	if err := os.WriteFile(kb, []byte(`{"submit":"ctrl+enter"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	pod := prepare(t, filepath.Join(t.TempDir(), "home"))
	if _, err := os.Stat(filepath.Join(pod, "keybindings.json")); err != nil {
		t.Fatalf("expected a staged copy first: %v", err)
	}

	if err := os.Remove(kb); err != nil { // the user drops their custom bindings
		t.Fatal(err)
	}
	prepare(t, pod) // same home, relaunched
	if _, err := os.Stat(filepath.Join(pod, "keybindings.json")); !os.IsNotExist(err) {
		t.Errorf("a stale copy outlived the host file, stat err = %v", err)
	}
}

// TestKeybindingsRestageOnRelaunch: copying instead of bind-mounting means a relaunch is what picks
// up an edit, so it has to actually pick it up.
func TestKeybindingsRestageOnRelaunch(t *testing.T) {
	hostDir := hostHome(t)
	kb := filepath.Join(hostDir, "keybindings.json")
	if err := os.WriteFile(kb, []byte(`{"submit":"ctrl+enter"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	pod := prepare(t, filepath.Join(t.TempDir(), "home"))

	const edited = `{"submit":"alt+enter"}`
	if err := os.WriteFile(kb, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	prepare(t, pod)
	got, err := os.ReadFile(filepath.Join(pod, "keybindings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != edited {
		t.Errorf("relaunch staged %q, want the edited %q", got, edited)
	}
}

// TestPrepareHomeStillWritesItsOwnFiles: keybindings are an addition, so the files the pod cannot
// start without must be untouched by it.
func TestPrepareHomeStillWritesItsOwnFiles(t *testing.T) {
	hostHome(t)
	dir := filepath.Join(t.TempDir(), "home")
	home, err := (Claude{}).PrepareHome(agent.HomeSpec{Dir: dir, SystemPrompt: "the prompt", Out: io.Discard})
	if err != nil {
		t.Fatalf("PrepareHome: %v", err)
	}
	for _, name := range []string{"settings.json", "system-prompt.txt"} {
		if _, serr := os.Stat(filepath.Join(dir, name)); serr != nil {
			t.Errorf("%s missing: %v", name, serr)
		}
	}
	if _, serr := os.Stat(home.ConfigPath); serr != nil {
		t.Errorf("config missing: %v", serr)
	}
}

// TestGoWorkspaceGetsTheGoLanguageServer: the wiring only matters if it lands in the file Claude
// actually reads. User-scope MCP servers live in ~/.claude.json under "mcpServers" — checked
// against `claude mcp add -s user`, since a wrong key fails silently and the whole point is to be
// there when the agent reaches for it.
func TestGoWorkspaceGetsTheGoLanguageServer(t *testing.T) {
	dir, ws := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	home, err := (Claude{}).PrepareHome(agent.HomeSpec{Dir: dir, SystemPrompt: "p", Out: io.Discard, Workspace: ws})
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	raw, err := os.ReadFile(home.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	servers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("no mcpServers in the config Claude reads:\n%s", raw)
	}
	gopls, ok := servers["gopls"].(map[string]any)
	if !ok {
		t.Fatalf("no gopls server declared: %v", servers)
	}
	// brokkr, not gopls directly: brokkr reaches the pod through the mounted pod-bin, so this is
	// fixable without an image rebuild, and its shim turns a refused toolchain into advice.
	if gopls["command"] != "brokkr" {
		t.Errorf("command = %v, want brokkr (the shim), not gopls itself", gopls["command"])
	}
	if gopls["type"] != "stdio" {
		t.Errorf("type = %v, want stdio", gopls["type"])
	}
	// The server runs in the pod, where the tree is mounted at /workspace — not at the host path
	// this decision was made from.
	args := fmt.Sprint(gopls["args"])
	if !strings.Contains(args, "gopls-mcp") || !strings.Contains(args, "/workspace") {
		t.Errorf("args = %v, want the shim pointed at the pod's /workspace", gopls["args"])
	}
	if strings.Contains(args, ws) {
		t.Errorf("args leak the host path %q — that path does not exist in the pod: %v", ws, gopls["args"])
	}
}

// TestNonGoWorkspaceIsUntouched: a pod with no Go project must be exactly as it was — no server
// declared, not even an empty key. Declaring one would start a language server for a language the
// project does not use.
func TestNonGoWorkspaceIsUntouched(t *testing.T) {
	for _, tc := range []struct{ name, ws string }{
		{"no go.mod", t.TempDir()},
		{"no workspace at all", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			home, err := (Claude{}).PrepareHome(agent.HomeSpec{Dir: dir, SystemPrompt: "p", Out: io.Discard, Workspace: tc.ws})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(home.ConfigPath)
			if err != nil {
				t.Fatal(err)
			}
			var cfg map[string]any
			if err := json.Unmarshal(raw, &cfg); err != nil {
				t.Fatal(err)
			}
			if _, present := cfg["mcpServers"]; present {
				t.Errorf("a non-Go workspace was given an MCP server:\n%s", raw)
			}
			// The files the pod cannot start without stay untouched either way.
			if cfg["hasCompletedOnboarding"] != true {
				t.Error("the onboarding flag was lost")
			}
		})
	}
}
