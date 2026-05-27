package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/container"
)

var NorseNames = []string{
	"brokkr", "dvalin", "alviss", "andvari", "eitri", "fjalar", "galar",
	"hreidmar", "ivaldi", "lit", "nordri", "sudri", "austri", "vestri",
	"regin", "motsoenir", "durin", "nyi", "thorin", "fili", "kili",
	"bombur", "nori", "ori", "gloin", "dori", "bifur", "bofur",
}

// GitRoot returns the git repository root for the current directory.
func GitRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// BaseBranch detects the main branch of the repository.
// Returns an error if the repo is in detached HEAD state.
func BaseBranch(projectRoot string) (string, error) {
	out, err := exec.Command("git", "-C", projectRoot, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("cannot detect base branch: %w", err)
	}
	b := strings.TrimSpace(string(out))
	if b == "" || b == "HEAD" {
		return "", fmt.Errorf("repository is in detached HEAD state — run 'git checkout master' on the main repo first")
	}
	return b, nil
}

// FindAvailable finds an idle worktree or creates one with the next available Norse name.
// Returns the name and whether it was newly created.
func FindAvailable(projectRoot string) (name string, created bool, err error) {
	workers := List(projectRoot)
	for _, wk := range workers {
		if wk.Role == "worker" && wk.Status == "-" {
			return wk.Name, false, nil
		}
	}

	if exec.Command("git", "-C", projectRoot, "rev-parse", "HEAD").Run() != nil {
		return "", false, fmt.Errorf("repo has no commits yet")
	}
	taken := make(map[string]bool)
	for _, wk := range workers {
		taken[wk.Name] = true
	}
	for _, n := range NorseNames {
		if !taken[n] {
			name = n
			break
		}
	}
	if name == "" {
		return "", false, fmt.Errorf("all Norse names taken")
	}
	if err := New(projectRoot, name); err != nil {
		return "", false, err
	}
	return name, true, nil
}

// New creates a git worktree with the given name under .worktrees/.
func New(projectRoot, name string) error {
	wtPath := projectRoot + "/.worktrees/" + name
	_ = os.MkdirAll(projectRoot+"/.worktrees", 0755)
	// Try creating with a new branch; if branch exists, use --detach
	out, err := exec.Command("git", "-C", projectRoot, "worktree", "add", "-b", name, wtPath, "HEAD").CombinedOutput()
	if err != nil {
		out, err = exec.Command("git", "-C", projectRoot, "worktree", "add", "--detach", wtPath, "HEAD").CombinedOutput()
	}
	if err != nil {
		return fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// StartOpts configures a worker or reviewer start.
type StartOpts struct {
	Skill string
	Shell bool
}

// Start launches a podman container for a worker worktree.
func Start(projectRoot, name string, opts StartOpts) error {
	wtPath := projectRoot + "/.worktrees/" + name
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		return fmt.Errorf("worktree %q not found", name)
	}

	cName := "sindri-" + name
	_ = exec.Command("podman", "rm", "-f", cName).Run()

	claudeHome, configPath := prepareClaudeHome(projectRoot, name)
	base, err := BaseBranch(projectRoot)
	if err != nil {
		return err
	}

	ghBin, err := findSindriGH()
	if err != nil {
		return err
	}

	podmanArgs := []string{
		"run", "--rm", "-it",
		"--name", cName,
		"--userns=keep-id",
		"--label", "sindri.project=" + projectRoot,
		"--label", "sindri.worker=" + name,
		"-v", claudeHome + ":/home/sindri/.claude:rw,z",
		"-v", configPath + ":/home/sindri/.claude.json:rw,z",
		"-v", ghBin + ":/opt/sindri/sindri-gh:ro,z",
		"-e", "GH_LOCAL_BASE=" + base,
		"-e", "COLORTERM=truecolor",
		"-e", "TD_ROOT=/project",
		"-v", projectRoot + "/.todos:/project/.todos:rw,z",
		"-v", wtPath + ":/workspace:rw,z",
		"-v", projectRoot + ":/repo:ro,z",
		"-v", projectRoot + "/.git:/repo/.git:rw,z",
		"-w", "/workspace",
		container.ImageName,
	}

	// Fix .git worktree pointer (may be broken from previous container kill)
	hostGitDir := fmt.Sprintf("gitdir: %s/.git/worktrees/%s\n", projectRoot, name)
	gitFile := wtPath + "/.git"
	if info, err := os.Stat(gitFile); err == nil && !info.IsDir() {
		_ = os.WriteFile(gitFile, []byte(hostGitDir), 0644)
	}

	// Detach at base branch tip so agent can create per-task branches.
	// Worktrees can't checkout a branch already used by another worktree.
	fmt.Fprintf(os.Stderr, "Detaching %s at %s...\n", name, base)
	if out, err := exec.Command("git", "-C", wtPath, "checkout", "--detach", base).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: detach failed: %s\n", strings.TrimSpace(string(out)))
	}

	// Container startup: rewrite .git for container paths
	containerGitDir := fmt.Sprintf("gitdir: /repo/.git/worktrees/%s", name)
	startup := "ln -sf /opt/sindri/sindri-gh /usr/local/bin/gh 2>/dev/null; " +
		"mkdir -p /home/sindri/.claude/skills && ln -sfn /opt/sindri/skills/* /home/sindri/.claude/skills/ 2>/dev/null; " +
		"ln -sf /opt/sindri/CLAUDE.md /workspace/CLAUDE.md 2>/dev/null; " +
		"if [ -f /workspace/.git ]; then " +
		fmt.Sprintf("echo '%s' > /workspace/.git; ", containerGitDir) +
		"fi; "

	// Restore host .git path after container exits
	defer os.WriteFile(gitFile, []byte(hostGitDir), 0644)

	skill := opts.Skill
	if skill == "" {
		skill = "td-next"
	}

	if opts.Shell {
		claudeCmd := fmt.Sprintf("claude --name %s /%s", name, skill)
		podmanArgs = append(podmanArgs, "bash", "-c",
			startup+fmt.Sprintf("echo 'Would launch: %s'; echo 'Skills:'; ls -la ~/.claude/skills/; exec bash", claudeCmd))
	} else {
		podmanArgs = append(podmanArgs, "bash", "-c",
			startup+fmt.Sprintf("exec claude --name %s /%s", name, skill))
	}

	proc := exec.Command("podman", podmanArgs...)
	proc.Stdin = os.Stdin
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	return proc.Run()
}

// StartReviewer launches the reviewer container.
func StartReviewer(projectRoot string, shell bool) error {
	cName := "sindri-reviewer"
	_ = exec.Command("podman", "rm", "-f", cName).Run()

	claudeHome, configPath := prepareClaudeHome(projectRoot, "reviewer")

	ghBin, err := findSindriGH()
	if err != nil {
		return err
	}

	podmanArgs := []string{
		"run", "--rm", "-it",
		"--name", cName,
		"--userns=keep-id",
		"--label", "sindri.project=" + projectRoot,
		"--label", "sindri.worker=_reviewer",
		"-v", claudeHome + ":/home/sindri/.claude:rw,z",
		"-v", configPath + ":/home/sindri/.claude.json:rw,z",
		"-v", ghBin + ":/opt/sindri/sindri-gh:ro,z",
		"-e", "TD_ROOT=/project",
		"-e", "COLORTERM=truecolor",
		"-v", projectRoot + "/.todos:/project/.todos:rw,z",
		"-v", projectRoot + ":/workspace:ro,z",
		"-v", projectRoot + "/.git:/workspace/.git:rw,z",
		"-w", "/workspace",
		container.ImageName,
	}

	startup := "ln -sf /opt/sindri/sindri-gh /usr/local/bin/gh 2>/dev/null; " +
		"mkdir -p /home/sindri/.claude/skills && ln -sfn /opt/sindri/skills/* /home/sindri/.claude/skills/ 2>/dev/null; " +
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

// Stop stops and removes a worker container by name.
func Stop(name string) error {
	cName := "sindri-" + name
	if name == "reviewer" || name == "_reviewer" {
		cName = "sindri-reviewer"
	}
	fmt.Fprintf(os.Stderr, "Stopping %s...\n", name)
	_ = exec.Command("podman", "stop", "-t", "3", cName).Run()
	out, err := exec.Command("podman", "rm", "-f", cName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed: %s", strings.TrimSpace(string(out)))
	}
	fmt.Fprintf(os.Stderr, "Worker %s stopped.\n", name)
	return nil
}

// Reset stops all running worker containers. Returns the number stopped.
func Reset(projectRoot string) (int, error) {
	workers := List(projectRoot)
	stopped := 0
	for _, wk := range workers {
		if wk.Container == "" {
			continue
		}
		fmt.Fprintf(os.Stderr, "Stopping %s...\n", wk.Name)
		_ = exec.Command("podman", "stop", "-t", "3", wk.Container).Run()
		_ = exec.Command("podman", "rm", "-f", wk.Container).Run()
		stopped++
	}
	return stopped, nil
}

// EnsureImage builds the container image if needed.
func EnsureImage(projectRoot string) error {
	return container.Ensure(projectRoot)
}

// findSindriGH locates the sindri-gh binary on the host.
func findSindriGH() (string, error) {
	// Check next to the running sindri binary first
	self, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(self), "sindri-gh")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	// Fall back to PATH
	if path, err := exec.LookPath("sindri-gh"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("sindri-gh binary not found — run 'make install'")
}

// prepareClaudeHome sets up the claude home directory with credentials and settings.
func prepareClaudeHome(projectRoot, name string) (claudeHome, configPath string) {
	claudeHome = projectRoot + "/.worktrees/.claude-home-" + name
	_ = os.MkdirAll(claudeHome, 0755)

	homeDir, _ := os.UserHomeDir()
	if data, err := os.ReadFile(homeDir + "/.claude/.credentials.json"); err == nil {
		_ = os.WriteFile(claudeHome+"/.credentials.json", data, 0600)
	}

	configPath = claudeHome + ".json"
	configData := map[string]interface{}{}
	if existing, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(existing, &configData)
	}
	configData["hasCompletedOnboarding"] = true
	configData["autoUpdates"] = false
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
  },
  "statusLine": {
    "type": "command",
    "command": "cat /tmp/claude-status 2>/dev/null || echo 'idle'",
    "refreshInterval": 5
  }
}`), 0644)

	return claudeHome, configPath
}
