// package: hub / claude
// type:    logic (agent runtime setup)
// job:     prepare an agent pod's Claude home — credentials, config, settings —
//          and build its role-aware system prompt. Claude runs interactively in
//          the pod's tmux session; its instructions come from the hub (served,
//          not baked), via this system prompt plus injected messages.
// limits:  no podman/tmux here (-> Launch wires the mounts and entrypoint).
package hub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/flo-at/sindri/internal/paths"
)

// claudeSettings grants the agent the tools it needs without per-call prompts.
const claudeSettings = `{
  "permissions": {
    "allow": ["Bash(*)","Read(*)","Edit(*)","Write(*)","Glob(*)","Grep(*)"],
    "defaultMode": "default"
  }
}`

// prepareClaudeHome sets up a per-agent Claude home under the central state dir
// (<state>/<project>/claude/<name>/, mounted at /home/sindri/.claude) and a config
// file (mounted at
// /home/sindri/.claude.json), seeding host credentials so the agent is
// authenticated. Returns the host paths to mount and whether credentials were
// found (no creds → caller should fall back to a shell).
func (h *Hub) prepareClaudeHome(project, name, role string, out io.Writer) (homeDir, configPath string, hasCreds bool, err error) {
	homeDir = filepath.Join(paths.StateDir(), project, "claude", name)
	if err = os.MkdirAll(homeDir, 0o755); err != nil {
		return "", "", false, fmt.Errorf("create claude home: %w", err)
	}
	if data, found := hostClaudeCredentials(out); found {
		if werr := os.WriteFile(filepath.Join(homeDir, ".credentials.json"), data, 0o600); werr == nil {
			hasCreds = true
		}
	}
	configPath = homeDir + ".json"
	// Trust is recorded per-project under projects["<dir>"].hasTrustDialogAccepted
	// — pre-accept /workspace so Claude doesn't block on the trust dialog.
	cfg, _ := json.Marshal(map[string]any{
		"hasCompletedOnboarding":        true,
		"autoUpdates":                   false,
		"bypassPermissionsModeAccepted": true, // pre-accept --dangerously-skip-permissions
		"theme":                         "dark-ansi", // built-in ANSI dark — readable in the pod's terminal, unlike the default
		"projects": map[string]any{
			"/workspace": map[string]any{"hasTrustDialogAccepted": true},
		},
	})
	if err = os.WriteFile(configPath, cfg, 0o644); err != nil {
		return "", "", false, fmt.Errorf("write claude config: %w", err)
	}
	if err = os.WriteFile(filepath.Join(homeDir, "settings.json"), []byte(claudeSettings), 0o644); err != nil {
		return "", "", false, fmt.Errorf("write claude settings: %w", err)
	}
	// Inject the project's architecture INTO the system prompt (full content, not just
	// a path), so every agent has it in context. Best-effort: a missing/unreadable doc
	// just omits the section (architectureBrief returns "" for empty content).
	archPath := h.architectureDoc(project)
	archContent, _ := os.ReadFile(filepath.Join(h.projectRoot(project), archPath))
	// brokkr is mounted into every pod (a cross-built linux binary, see Launch), so the
	// brief always recommends it.
	if err = os.WriteFile(filepath.Join(homeDir, "system-prompt.txt"), []byte(systemPrompt(name, role, string(archContent), archPath)), 0o644); err != nil {
		return "", "", false, fmt.Errorf("write system prompt: %w", err)
	}
	return homeDir, configPath, hasCreds, nil
}

// hostClaudeCredentials returns the user's Claude Code OAuth credentials (the JSON
// the pod expects at ~/.claude/.credentials.json), or ok=false when none exist.
// Claude Code keeps them in a file on Linux but in the macOS Keychain (a "Claude
// Code-credentials" generic-password item), so on macOS we fall back to reading
// the Keychain — announcing that on w, since it may pop a one-time "Allow" prompt.
func hostClaudeCredentials(w io.Writer) (data []byte, ok bool) {
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
