// package: adapter/agent/claude / home
// type:    adapter (Claude Code — provisions a pod's Claude home)
// job:     write a per-agent Claude home the pod mounts — seed host credentials
//
//	(file, or the macOS Keychain), the config that pre-accepts onboarding +
//	the /workspace trust dialog, the tool-permission settings, and the
//	workflow-composed system prompt. The mechanics behind agent.PrepareHome.
//
// limits:  Claude-specific files only; the system prompt is handed in already
//
//	composed (the workflow owns that logic), and WHERE the home lives is the
//	hub's call (spec.Dir). No podman/tmux (-> the hub wires the mounts).
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

// PrepareHome sets up a per-agent Claude home under spec.Dir (mounted at
// /home/sindri/.claude) and a sibling config file (spec.Dir+".json", mounted at
// /home/sindri/.claude.json), seeding host credentials so the agent is authenticated.
// The system prompt is persisted verbatim (the workflow already composed it). HasCreds
// is false when no host credentials were found — the caller then falls back to a shell.
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
	cfg, _ := json.Marshal(map[string]any{
		"hasCompletedOnboarding":        true,
		"autoUpdates":                   false,
		"bypassPermissionsModeAccepted": true,        // pre-accept --dangerously-skip-permissions
		"theme":                         "dark-ansi", // built-in ANSI dark — readable in the pod's terminal, unlike the default
		"projects": map[string]any{
			"/workspace": map[string]any{"hasTrustDialogAccepted": true},
		},
	})
	if err := os.WriteFile(configPath, cfg, 0o644); err != nil {
		return agent.Home{}, fmt.Errorf("write claude config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(spec.Dir, "settings.json"), []byte(settings), 0o644); err != nil {
		return agent.Home{}, fmt.Errorf("write claude settings: %w", err)
	}
	if err := os.WriteFile(filepath.Join(spec.Dir, "system-prompt.txt"), []byte(spec.SystemPrompt), 0o644); err != nil {
		return agent.Home{}, fmt.Errorf("write system prompt: %w", err)
	}
	return agent.Home{Dir: spec.Dir, ConfigPath: configPath, HasCreds: hasCreds}, nil
}

// hostCredentials returns the user's Claude Code OAuth credentials (the JSON the pod
// expects at ~/.claude/.credentials.json), or ok=false when none exist. Claude Code
// keeps them in a file on Linux but in the macOS Keychain (a "Claude Code-credentials"
// generic-password item), so on macOS we fall back to reading the Keychain —
// announcing that on w, since it may pop a one-time "Allow" prompt.
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
