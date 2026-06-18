// package: hub / claude
// type:    logic (agent runtime setup)
// job:     prepare an agent pod's Claude home — credentials, config, settings —
//          and build its role-aware system prompt. Claude runs interactively in
//          the pod's tmux session; its instructions come from the hub (served,
//          not baked), via this system prompt plus injected messages.
// limits:  no podman/tmux here (-> Launch wires the mounts and entrypoint).
package hub

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// claudeSettings grants the agent the tools it needs without per-call prompts.
const claudeSettings = `{
  "permissions": {
    "allow": ["Bash(*)","Read(*)","Edit(*)","Write(*)","Glob(*)","Grep(*)"],
    "defaultMode": "default"
  }
}`

// prepareClaudeHome sets up a per-agent Claude home under .sindri/claude/<name>/
// (mounted at /home/sindri/.claude) and a config file (mounted at
// /home/sindri/.claude.json), seeding host credentials so the agent is
// authenticated. Returns the host paths to mount and whether credentials were
// found (no creds → caller should fall back to a shell).
func (h *Hub) prepareClaudeHome(name, role string) (homeDir, configPath string, hasCreds bool, err error) {
	homeDir = filepath.Join(h.root, ".sindri", "claude", name)
	if err = os.MkdirAll(homeDir, 0o755); err != nil {
		return "", "", false, fmt.Errorf("create claude home: %w", err)
	}
	if host, herr := os.UserHomeDir(); herr == nil {
		if data, rerr := os.ReadFile(filepath.Join(host, ".claude", ".credentials.json")); rerr == nil {
			if werr := os.WriteFile(filepath.Join(homeDir, ".credentials.json"), data, 0o600); werr == nil {
				hasCreds = true
			}
		}
	}
	configPath = homeDir + ".json"
	// Trust is recorded per-project under projects["<dir>"].hasTrustDialogAccepted
	// — pre-accept /workspace so Claude doesn't block on the trust dialog.
	cfg, _ := json.Marshal(map[string]any{
		"hasCompletedOnboarding":        true,
		"autoUpdates":                   false,
		"bypassPermissionsModeAccepted": true, // pre-accept --dangerously-skip-permissions
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
	if err = os.WriteFile(filepath.Join(homeDir, "system-prompt.txt"), []byte(systemPrompt(name, role)), 0o644); err != nil {
		return "", "", false, fmt.Errorf("write system prompt: %w", err)
	}
	return homeDir, configPath, hasCreds, nil
}
