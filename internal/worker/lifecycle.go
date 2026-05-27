package worker

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
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
func BaseBranch(projectRoot string) string {
	branch := "master"
	if out, err := exec.Command("git", "-C", projectRoot, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	}
	return branch
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
	out, err := exec.Command("git", "-C", projectRoot, "worktree", "add", "-b", name, wtPath, "HEAD").CombinedOutput()
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
	image := "sindri-agent:test"
	base := BaseBranch(projectRoot)

	podmanArgs := []string{
		"run", "--rm", "-it",
		"--name", cName,
		"--userns=keep-id",
		"--label", "sindri.project=" + projectRoot,
		"--label", "sindri.worker=" + name,
		"-v", claudeHome + ":/home/sindri/.claude:rw,z",
		"-v", configPath + ":/home/sindri/.claude.json:rw,z",
		"-e", "GH_LOCAL_BASE=" + base,
		"-e", "COLORTERM=truecolor",
		"-e", "TD_ROOT=/project",
		"-v", projectRoot + "/.todos:/project/.todos:rw,z",
		"-v", wtPath + ":/workspace:rw,z",
		"-v", projectRoot + ":/repo:ro,z",
		"-v", projectRoot + "/.git:/repo/.git:rw,z",
		"-w", "/workspace",
		image,
	}

	// Fix .git worktree pointer (may be broken from previous container kill)
	hostGitDir := fmt.Sprintf("gitdir: %s/.git/worktrees/%s\n", projectRoot, name)
	gitFile := wtPath + "/.git"
	if info, err := os.Stat(gitFile); err == nil && !info.IsDir() {
		_ = os.WriteFile(gitFile, []byte(hostGitDir), 0644)
	}

	// Rebase onto base branch before entering container
	fmt.Fprintf(os.Stderr, "Rebasing %s onto %s...\n", name, base)
	if out, err := exec.Command("git", "-C", wtPath, "rebase", base).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: rebase failed: %s\n", strings.TrimSpace(string(out)))
	}

	// Container startup: rewrite .git for container paths
	containerGitDir := fmt.Sprintf("gitdir: /repo/.git/worktrees/%s", name)
	startup := "mkdir -p /home/sindri/.claude/skills && ln -sfn /opt/sindri/skills/* /home/sindri/.claude/skills/ 2>/dev/null; " +
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
	dockerfile := projectRoot + "/container/Dockerfile"
	content, err := os.ReadFile(dockerfile)
	if err != nil {
		if exec.Command("podman", "image", "exists", "sindri-agent:test").Run() == nil {
			return nil
		}
		return fmt.Errorf("no Dockerfile and no sindri-agent:test image")
	}

	year, week := time.Now().ISOWeek()
	h := sha256.New()
	h.Write(content)
	h.Write([]byte(fmt.Sprintf("%d-%d", year, week)))
	buildKey := fmt.Sprintf("%x", h.Sum(nil))[:16]

	cacheFile := projectRoot + "/.worktrees/.build-key"
	if cached, err := os.ReadFile(cacheFile); err == nil && strings.TrimSpace(string(cached)) == buildKey {
		return nil
	}

	fmt.Fprintf(os.Stderr, "Building container image...\n")
	_ = os.MkdirAll(projectRoot+"/bin", 0755)
	for _, bin := range []string{"td", "yq"} {
		if path, err := exec.LookPath(bin); err == nil {
			data, _ := os.ReadFile(path)
			_ = os.WriteFile(projectRoot+"/bin/"+bin, data, 0755)
		}
	}

	cmd := exec.Command("podman", "build", "-t", "sindri-agent:test", "-f", dockerfile, projectRoot)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("image build failed: %w", err)
	}

	_ = os.MkdirAll(projectRoot+"/.worktrees", 0755)
	_ = os.WriteFile(cacheFile, []byte(buildKey), 0644)
	return nil
}

// LoadSkill reads a skill from /opt/sindri/skills/<name>/SKILL.md inside the container image.
func LoadSkill(image, name string) (string, error) {
	path := "/opt/sindri/skills/" + name + "/SKILL.md"
	out, err := exec.Command("podman", "run", "--rm", image, "cat", path).Output()
	if err != nil {
		listOut, _ := exec.Command("podman", "run", "--rm", image, "ls", "/opt/sindri/skills/").Output()
		var available []string
		for _, line := range strings.Split(strings.TrimSpace(string(listOut)), "\n") {
			if strings.HasSuffix(line, ".md") {
				available = append(available, strings.TrimSuffix(line, ".md"))
			}
		}
		return "", fmt.Errorf("skill %q not found. Available: %s", name, strings.Join(available, ", "))
	}
	return strings.TrimSpace(string(out)), nil
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
  }
}`), 0644)

	return claudeHome, configPath
}
