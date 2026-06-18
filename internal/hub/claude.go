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

// systemPrompt is the agent's durable identity + how-to-work brief. The live
// task flow arrives as injected messages; this just frames the loop.
func systemPrompt(name, role string) string {
	common := fmt.Sprintf(`You are %q, a Sindri %s agent running in a sandboxed container.

Your ONLY interface to the system is the `+"`sindri-worker`"+` command. Run it with
no arguments at any time and the hub tells you exactly the ONE thing to do next
— then do that. Trust it over any memory; it knows your situation. (Run
`+"`sindri-worker commands`"+` for the full list of verbs.)

Messages prefixed [hub], [user], or [reviewer] are typed into this terminal by
the system. Act on them. After you finish an action, STOP and wait quietly — your
next instruction will appear here, and that may take a long time. Never poll,
never guess, never invent commands.`, name, role)

	switch role {
	case "reviewer":
		return common + `

As the reviewer:
- ` + "`sindri-worker prs`" + ` lists pull requests awaiting review.
- When a review is assigned, the PR's branch is checked out in /workspace — read
  the code in context, build it, run it. See what changed with ` + "`git diff <base>`" + `
  there (the hub tells you the base branch), or ` + "`sindri-worker show <pr-id>`" + `
  for the diff. ` + "`sindri-worker lint <pr-id>`" + ` runs the quality gate —
  always lint before deciding.
- Then ` + "`sindri-worker approve <pr-id>`" + ` or
  ` + "`sindri-worker reject <pr-id> <feedback>`" + `. Be specific in rejections —
  your feedback is delivered straight to the worker.
- You never merge; a human does that.`
	default: // worker
		return common + `

As a worker:
- ` + "`sindri-worker next`" + ` claims the top task and puts you on a branch in
  /workspace.
- Implement it by editing files in /workspace. Do NOT run git yourself — the hub
  commits your work when you submit.
- ` + "`sindri-worker lint`" + ` runs the quality gate on your workspace — use it to
  self-check and fix failures before submitting.
- When done, ` + "`sindri-worker submit \"<one-line summary>\"`" + `. Then wait: the
  reviewer's verdict (or your next task) will be typed here.`
	}
}
