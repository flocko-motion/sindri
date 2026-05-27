package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func newReviewCmd() *cobra.Command {
	var shellMode bool
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Start a reviewer (alias for 'worker review')",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReview(shellMode)
		},
	}
	cmd.Flags().BoolVar(&shellMode, "shell", false, "Open a shell instead of launching claude (for debugging)")
	return cmd
}

func runReview(shell bool) error {
	projectRoot, err := gitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	if err := ensureImage(projectRoot); err != nil {
		return err
	}

	cName := "sindri-reviewer"
	_ = exec.Command("podman", "rm", "-f", cName).Run()

	claudeHome := projectRoot + "/.worktrees/.claude-home-reviewer"
	_ = os.MkdirAll(claudeHome, 0755)
	homeDir, _ := os.UserHomeDir()
	if data, err := os.ReadFile(homeDir + "/.claude/.credentials.json"); err == nil {
		_ = os.WriteFile(claudeHome+"/.credentials.json", data, 0600)
	}

	configPath := claudeHome + ".json"
	configData := map[string]interface{}{}
	if existing, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(existing, &configData)
	}
	configData["hasCompletedOnboarding"] = true
	trustedDirs, _ := configData["trustedDirectories"].(map[string]interface{})
	if trustedDirs == nil {
		trustedDirs = map[string]interface{}{}
	}
	trustedDirs["/workspace"] = true
	configData["trustedDirectories"] = trustedDirs
	configJSON, _ := json.Marshal(configData)
	_ = os.WriteFile(configPath, configJSON, 0644)

	settingsPath := claudeHome + "/settings.json"
	_ = os.WriteFile(settingsPath, []byte(`{
  "permissions": {
    "allow": [
      "Bash(*)",
      "Read(*)",
      "Edit(*)",
      "Write(*)",
      "Glob(*)",
      "Grep(*)",
      "WebFetch(*)",
      "WebSearch(*)",
      "NotebookEdit(*)"
    ],
    "defaultMode": "default"
  }
}`), 0644)

	image := "sindri-agent:test"

	podmanArgs := []string{
		"run", "--rm", "-it",
		"--name", cName,
		"--userns=keep-id",
		"--label", "sindri.project=" + projectRoot,
		"--label", "sindri.worker=_reviewer",
		"-v", claudeHome + ":/home/sindri/.claude:rw,z",
		"-v", configPath + ":/home/sindri/.claude.json:rw,z",
		"-e", "TD_ROOT=/project",
		"-e", "COLORTERM=truecolor",
		"-v", projectRoot + "/.todos:/project/.todos:rw,z",
		"-v", projectRoot + ":/workspace:ro,z",
		"-v", projectRoot + "/.git:/workspace/.git:rw,z",
		"-w", "/workspace",
		image,
	}

	startup := "mkdir -p /home/sindri/.claude/skills && ln -sfn /opt/sindri/skills/* /home/sindri/.claude/skills/ 2>/dev/null; " +
		"cp /opt/sindri/CLAUDE.reviewer.md /workspace/CLAUDE.md 2>/dev/null || ln -sf /opt/sindri/CLAUDE.reviewer.md /workspace/CLAUDE.md 2>/dev/null; "

	skill := "td-review"
	if shell {
		claudeCmd := fmt.Sprintf("claude --name reviewer /%s", skill)
		podmanArgs = append(podmanArgs, "bash", "-c",
			startup+fmt.Sprintf("echo 'Would launch: %s'; echo 'Skills:'; ls -la ~/.claude/skills/; exec bash", claudeCmd))
	} else {
		podmanArgs = append(podmanArgs, "bash", "-c",
			startup+fmt.Sprintf("exec claude --name reviewer /%s", skill))
	}

	fmt.Fprintf(os.Stderr, "Starting reviewer...\n")
	proc := exec.Command("podman", podmanArgs...)
	proc.Stdin = os.Stdin
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	return proc.Run()
}
