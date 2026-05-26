package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)


func main() {
	var projectDir string
	rootCmd := &cobra.Command{
		Use:   "sindri",
		Short: "Sindri — AI agent orchestrator",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if projectDir != "" {
				return os.Chdir(projectDir)
			}
			return nil
		},
	}
	rootCmd.PersistentFlags().StringVar(&projectDir, "project", "", "Project directory (default: git root from cwd)")

	workerCmd := &cobra.Command{
		Use:   "worker",
		Short: "Manage Sindri workers",
	}

	workerListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all workers and their status",
		RunE:  runWorkerList,
	}

	var skillName string
	var shellMode bool
	workerStartCmd := &cobra.Command{
		Use:   "start [name]",
		Short: "Start a worker interactively (creates worktree if needed)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkerStart(cmd, args, skillName, shellMode)
		},
	}
	workerStartCmd.Flags().StringVar(&skillName, "skill", "", "Skill to run (e.g. td-next, td-review)")
	workerStartCmd.Flags().BoolVar(&shellMode, "shell", false, "Open a shell instead of launching claude (for debugging)")

	workerStopCmd := &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a running worker container",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkerStop,
	}

	workerCmd.AddCommand(workerListCmd, workerStartCmd, workerStopCmd)
	rootCmd.AddCommand(workerCmd)

	// Top-level alias: sindri work = sindri worker start
	var workSkill string
	var workShell bool
	workCmd := &cobra.Command{
		Use:   "work [name]",
		Short: "Start a worker (alias for 'worker start')",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkerStart(cmd, args, workSkill, workShell)
		},
	}
	workCmd.Flags().StringVar(&workSkill, "skill", "", "Skill to run (e.g. td-next, td-review)")
	workCmd.Flags().BoolVar(&workShell, "shell", false, "Open a shell instead of launching claude")
	rootCmd.AddCommand(workCmd)

	rootCmd.AddCommand(newTuiCmd())
	rootCmd.AddCommand(newPrCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// ── Norse names ─────────────────────────────────────────────────────────────

var norseNames = []string{
	"brokkr", "dvalin", "alviss", "andvari", "eitri", "fjalar", "galar",
	"hreidmar", "ivaldi", "lit", "nordri", "sudri", "austri", "vestri",
	"regin", "motsoenir", "durin", "nyi", "thorin", "fili", "kili",
	"bombur", "nori", "ori", "gloin", "dori", "bifur", "bofur",
}

// ── worker start ────────────────────────────────────────────────────────────

func runWorkerStart(cmd *cobra.Command, args []string, skill string, shell bool) error {
	projectRoot, err := gitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	if err := ensureImage(projectRoot); err != nil {
		return err
	}

	var name string
	if len(args) > 0 {
		name = args[0]
	}

	// If no name given, find an unattached worktree or create a new one
	if name == "" {
		wtOut, _ := exec.Command("git", "-C", projectRoot, "worktree", "list", "--porcelain").Output()
		worktrees := parseWorktreeNames(string(wtOut), projectRoot)

		// Query running containers
		cOut, _ := exec.Command("podman", "ps", "--filter", "label=sindri.project="+projectRoot, "--format", "json").Output()
		running := make(map[string]bool)
		var containers []podmanContainer
		if len(strings.TrimSpace(string(cOut))) > 0 {
			_ = json.Unmarshal(cOut, &containers)
		}
		for _, c := range containers {
			if w := c.Labels["sindri.worker"]; w != "" {
				running[w] = true
			}
		}

		// Find an unattached worktree
		for wtName := range worktrees {
			if wtName == "main" {
				continue
			}
			if !running[wtName] {
				name = wtName
				fmt.Fprintf(os.Stderr, "🔨 resuming %s\n", name)
				break
			}
		}

		// None found — create a new one
		if name == "" {
			if exec.Command("git", "-C", projectRoot, "rev-parse", "HEAD").Run() != nil {
				return fmt.Errorf("repo has no commits yet")
			}
			taken := make(map[string]bool)
			for n := range worktrees {
				taken[n] = true
			}
			for _, n := range norseNames {
				if !taken[n] {
					name = n
					break
				}
			}
			if name == "" {
				return fmt.Errorf("all Norse names taken")
			}
			wtPath := projectRoot + "/.worktrees/" + name
			_ = os.MkdirAll(projectRoot+"/.worktrees", 0755)
			out, err := exec.Command("git", "-C", projectRoot, "worktree", "add", "-b", name, wtPath, "HEAD").CombinedOutput()
			if err != nil {
				return fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
			}
			fmt.Fprintf(os.Stderr, "🔨 %s created\n", name)
		}
	}

	wtPath := projectRoot + "/.worktrees/" + name
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		return fmt.Errorf("worktree %q not found", name)
	}

	cName := "sindri-" + name
	_ = exec.Command("podman", "rm", "-f", cName).Run()

	// Prepare claude home with credentials, settings, and skills
	claudeHome := projectRoot + "/.worktrees/.claude-home-" + name
	_ = os.MkdirAll(claudeHome, 0755)
	homeDir, _ := os.UserHomeDir()
	if data, err := os.ReadFile(homeDir + "/.claude/.credentials.json"); err == nil {
		_ = os.WriteFile(claudeHome+"/.credentials.json", data, 0600)
	}

	// Per-worker config at claudeHome/../.claude-<name>.json
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

	// Settings — pre-grant permissions for /workspace
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
		"--label", "sindri.worker=" + name,
		"-v", claudeHome + ":/home/sindri/.claude:rw,z",
		"-v", configPath + ":/home/sindri/.claude.json:rw,z",
		"-e", "GH_LOCAL_BASE=main",
		"-e", "TD_ROOT=/project",
		"-v", projectRoot + "/.todos:/project/.todos:rw,z",
		"-v", wtPath + ":/workspace:rw,z",
		"-v", projectRoot + ":/repo:ro,z",
		"-v", projectRoot + "/.git:/repo/.git:rw,z",
		"-w", "/workspace",
		image,
	}

	// Container startup: fix git worktree path + link skills
	// Save original .git, rewrite for container paths, restore on exit
	startup := "mkdir -p /home/sindri/.claude/skills && ln -sfn /opt/sindri/skills/* /home/sindri/.claude/skills/ 2>/dev/null; " +
		"ln -sf /opt/sindri/CLAUDE.md /workspace/CLAUDE.md 2>/dev/null; " +
		"cp /workspace/.git /tmp/.git.bak 2>/dev/null; " +
		fmt.Sprintf("echo 'gitdir: /repo/.git/worktrees/%s' > /workspace/.git; ", name) +
		"trap 'cp /tmp/.git.bak /workspace/.git 2>/dev/null' EXIT; "

	if shell {
		if skill == "" {
			skill = "td-next"
		}
		claudeCmd := fmt.Sprintf("claude --name %s /%s", name, skill)
		podmanArgs = append(podmanArgs, "bash", "-c",
			startup+fmt.Sprintf("echo 'Would launch: %s'; echo 'Skills:'; ls -la ~/.claude/skills/; exec bash", claudeCmd))
	} else {
		if skill == "" {
			skill = "td-next"
		}
		podmanArgs = append(podmanArgs, "bash", "-c",
			startup+fmt.Sprintf("exec claude --name %s /%s", name, skill))
	}

	proc := exec.Command("podman", podmanArgs...)
	proc.Stdin = os.Stdin
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	return proc.Run()
}

// ── worker list ─────────────────────────────────────────────────────────────

type podmanContainer struct {
	Names  []string          `json:"Names"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
}

func runWorkerList(cmd *cobra.Command, args []string) error {
	projectRoot, err := gitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	// Query containers
	out, err := exec.Command("podman", "ps", "-a",
		"--filter", "label=sindri.project="+projectRoot,
		"--format", "json",
	).Output()
	if err != nil {
		return fmt.Errorf("podman ps: %w", err)
	}

	var containers []podmanContainer
	if len(strings.TrimSpace(string(out))) > 0 {
		_ = json.Unmarshal(out, &containers)
	}

	containerByWorker := make(map[string]*podmanContainer)
	for i, c := range containers {
		if w := c.Labels["sindri.worker"]; w != "" {
			containerByWorker[w] = &containers[i]
		}
	}

	// Query worktrees
	wtOut, _ := exec.Command("git", "-C", projectRoot, "worktree", "list", "--porcelain").Output()
	worktrees := parseWorktreeNames(string(wtOut), projectRoot)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tROLE\tSTATUS")
	fmt.Fprintln(w, "----\t----\t------")

	// Main repo
	mainName := "sindri"
	if p, ok := worktrees["main"]; ok {
		parts := strings.Split(p, "/")
		mainName = parts[len(parts)-1]
	}
	if c, ok := containerByWorker["_reviewer"]; ok {
		fmt.Fprintf(w, "👑 %s\treviewer\t%s\n", mainName, c.State)
	} else {
		fmt.Fprintf(w, "👑 %s\treviewer\t-\n", mainName)
	}

	// Workers
	for name := range worktrees {
		if name == "main" {
			continue
		}
		if c, ok := containerByWorker[name]; ok {
			fmt.Fprintf(w, "🔨 %s\tworker\t%s\n", name, c.State)
		} else {
			fmt.Fprintf(w, "🔨 %s\tworker\t-\n", name)
		}
	}

	w.Flush()
	return nil
}

// ── worker stop ─────────────────────────────────────────────────────────────

func runWorkerStop(cmd *cobra.Command, args []string) error {
	name := args[0]
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

// ── helpers ─────────────────────────────────────────────────────────────────

func gitRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func parseWorktreeNames(output, mainDir string) map[string]string {
	names := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			path := strings.TrimPrefix(line, "worktree ")
			name := "main"
			if path != mainDir {
				parts := strings.Split(path, "/")
				name = parts[len(parts)-1]
			}
			names[name] = path
		}
	}
	return names
}

// loadSkill reads a skill from /opt/sindri/skills/<name>.md inside the container image.
func loadSkill(image, name string) (string, error) {
	path := "/opt/sindri/skills/" + name + "/SKILL.md"
	out, err := exec.Command("podman", "run", "--rm", image, "cat", path).Output()
	if err != nil {
		// List available
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

func ensureImage(projectRoot string) error {
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
