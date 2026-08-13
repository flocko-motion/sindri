// package: adapter/agent/claude / home
// type:    adapter (Claude Code — provisions a pod's Claude home)
// job:     write a per-agent Claude home the pod mounts — seed host credentials
// (file, or the macOS Keychain), the config that pre-accepts onboarding +
// the /workspace trust dialog, the tool-permission settings, and the
// workflow-composed system prompt. The mechanics behind agent.PrepareHome.
// limits:  Claude-specific files only; the system prompt is handed in already
// composed (the workflow owns that logic), and WHERE the home lives is the
// hub's call (spec.Dir). No podman/tmux (-> the hub wires the mounts).
package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/flo-at/sindri/internal/adapter/agent"
)

// settings grants the agent the tools it needs without per-call permission prompts.
const settings = `{
  "permissions": {
    "allow": ["Bash(*)","Read(*)","Edit(*)","Write(*)","Glob(*)","Grep(*)"],
    "defaultMode": "default"
  }
}`

// PrepareHome writes a per-agent Claude home under spec.Dir (mounted at /home/sindri/.claude) and a
// sibling config at spec.Dir+".json" (/home/sindri/.claude.json), seeding host credentials so the
// agent is authenticated. HasCreds is false when the host has none — the caller falls back to a shell.
func (Claude) PrepareHome(spec agent.HomeSpec) (agent.Home, error) {
	if err := os.MkdirAll(spec.Dir, 0o755); err != nil {
		return agent.Home{}, fmt.Errorf("create claude home: %w", err)
	}
	hasCreds := false
	if data, found := hostCredentials(spec.Out); found {
		if werr := os.WriteFile(filepath.Join(spec.Dir, ".credentials.json"), data, 0o600); werr == nil {
			hasCreds = true
		}
	}
	configPath := spec.Dir + ".json"
	// Trust is recorded per-project under projects["<dir>"].hasTrustDialogAccepted —
	// pre-accept /workspace so Claude doesn't block on the trust dialog.
	conf := map[string]any{
		"hasCompletedOnboarding":        true,
		"autoUpdates":                   false,
		"bypassPermissionsModeAccepted": true,        // pre-accept --dangerously-skip-permissions
		"theme":                         "dark-ansi", // built-in ANSI dark — readable in the pod's terminal, unlike the default
		"projects": map[string]any{
			"/workspace": map[string]any{"hasTrustDialogAccepted": true},
		},
	}
	conf["mcpServers"] = mcpServers()
	cfg, _ := json.Marshal(conf)
	if err := os.WriteFile(configPath, cfg, 0o644); err != nil {
		return agent.Home{}, fmt.Errorf("write claude config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(spec.Dir, "settings.json"), []byte(settings), 0o644); err != nil {
		return agent.Home{}, fmt.Errorf("write claude settings: %w", err)
	}
	if err := os.WriteFile(filepath.Join(spec.Dir, "system-prompt.txt"), []byte(spec.SystemPrompt), 0o644); err != nil {
		return agent.Home{}, fmt.Errorf("write system prompt: %w", err)
	}
	stageKeybindings(spec.Dir, spec.Out)
	return agent.Home{Dir: spec.Dir, ConfigPath: configPath, HasCreds: hasCreds}, nil
}

// mcpServers declares every pod's language tooling, under the "mcpServers" key
// `claude mcp add -s user` writes — a wrong one fails silently. UNCONDITIONAL is the point: a
// condition here read the workspace as it was when the home was prepared, so a module below the
// root, or a worktree not yet populated, lost the tooling for good. What the tree holds is the
// shim's question (-> brokkr gopls-mcp), answered per call and out loud.
func mcpServers() map[string]any {
	return map[string]any{
		"gopls": map[string]any{
			"type":    "stdio",
			"command": "brokkr",
			// The pod mounts the tree at /workspace, not at the host path decided from here.
			"args": []string{"gopls-mcp", "--dir", "/workspace"},
			"env":  map[string]any{},
		},
	}
}

// stageKeybindings copies the user's ~/.claude/keybindings.json in; absence removes a copy from an
// earlier launch, which this per-agent home would otherwise keep once the host file is gone.
//
// Copied rather than bind-mounted like skills: a per-file bind pins the inode and editors save by
// rename, so the pod would keep serving the pre-edit file (as the socket and pod-bin mounts warn).
func stageKeybindings(dir string, out io.Writer) {
	dst := filepath.Join(dir, "keybindings.json")
	data, found := hostClaudeFile("keybindings.json")
	if !found {
		_ = os.Remove(dst)
		return
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		fmt.Fprintf(out, "could not stage keybindings.json: %v\n", err)
	}
}

// hostClaudeFile reads a file from the user's own ~/.claude; ok is false when there is none.
func hostClaudeFile(name string) (data []byte, ok bool) {
	host, err := os.UserHomeDir()
	if err != nil {
		return nil, false
	}
	data, err = os.ReadFile(filepath.Join(host, ".claude", name))
	return data, err == nil
}

// hostCredentials returns the user's Claude Code OAuth credentials, or ok=false when none exist.
// Linux keeps them at ~/.claude/.credentials.json, macOS in the Keychain ("Claude Code-credentials",
// a generic-password item), so darwin falls back to that — announced on w: it may prompt for access.
func hostCredentials(w io.Writer) (data []byte, ok bool) {
	if host, err := os.UserHomeDir(); err == nil {
		if data, err := os.ReadFile(filepath.Join(host, ".claude", ".credentials.json")); err == nil {
			return data, true
		}
	}
	if runtime.GOOS == "darwin" {
		fmt.Fprintln(w, "macOS: no ~/.claude/.credentials.json — reading Claude credentials from the Keychain (may prompt for access)…")
		raw, err := exec.Command("security", "find-generic-password", "-s", "Claude Code-credentials", "-w").Output()
		if raw = bytes.TrimSpace(raw); err == nil && len(raw) > 0 {
			fmt.Fprintln(w, "macOS: loaded Claude credentials from the Keychain.")
			return raw, true
		}
		fmt.Fprintf(w, "macOS: could not read Claude credentials from the Keychain: %v\n", err)
	}
	return nil, false
}
