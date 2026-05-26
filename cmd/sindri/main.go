package main

import (
	"bufio"
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
	rootCmd := &cobra.Command{
		Use:   "sindri",
		Short: "Sindri — AI agent orchestrator",
	}

	workerCmd := &cobra.Command{
		Use:   "worker",
		Short: "Manage Sindri workers",
	}

	workerListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all Sindri workers (containers + worktrees)",
		RunE:  runWorkerList,
	}

	var rawOutput bool
	workerStreamCmd := &cobra.Command{
		Use:   "stream <name>",
		Short: "Stream live output from a worker",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkerStream(cmd, args, rawOutput)
		},
	}
	workerStreamCmd.Flags().BoolVar(&rawOutput, "raw", false, "Print raw stream-json without formatting")

	var startStream bool
	workerStartCmd := &cobra.Command{
		Use:   "start <name>",
		Short: "Start a Sindri agent in an existing worktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkerStart(cmd, args, startStream)
		},
	}
	workerStartCmd.Flags().BoolVar(&startStream, "stream", false, "Stream output after starting")

	workerInputCmd := &cobra.Command{
		Use:   "input <name> <message>",
		Short: "Send a message to a running worker's Claude session",
		Args:  cobra.ExactArgs(2),
		RunE:  runWorkerInput,
	}

	workerStopCmd := &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a running worker",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkerStop,
	}

	workerCmd.AddCommand(workerListCmd, workerStartCmd, workerStopCmd, workerStreamCmd, workerInputCmd)
	rootCmd.AddCommand(workerCmd)
	rootCmd.AddCommand(newWatchCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

type podmanContainer struct {
	Names  []string          `json:"Names"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
}

func runWorkerList(cmd *cobra.Command, args []string) error {
	// Get project root
	projectRoot, err := gitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	// Query podman for sindri containers
	out, err := exec.Command("podman", "ps", "-a",
		"--filter", "label=sindri.project="+projectRoot,
		"--format", "json",
	).Output()
	if err != nil {
		return fmt.Errorf("podman ps failed: %w", err)
	}

	var containers []podmanContainer
	if len(strings.TrimSpace(string(out))) > 0 {
		if err := json.Unmarshal(out, &containers); err != nil {
			return fmt.Errorf("parse podman output: %w", err)
		}
	}

	// Query git worktrees
	wtOut, _ := exec.Command("git", "-C", projectRoot, "worktree", "list", "--porcelain").Output()
	worktrees := parseWorktreeNames(string(wtOut), projectRoot)

	// Query td for in-progress issues
	tdIssues := getTdInProgress(projectRoot)

	// Build combined view
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tROLE\tSTATUS\tTASK")
	fmt.Fprintln(w, "----\t----\t------\t----")

	// Track which workers have containers
	containerByWorker := make(map[string]*podmanContainer)
	for i, c := range containers {
		worker := c.Labels["sindri.worker"]
		if worker != "" {
			containerByWorker[worker] = &containers[i]
		}
	}

	// Main repo = reviewer
	mainPath := worktrees["main"]
	name := "sindri"
	if mainPath != "" {
		parts := strings.Split(mainPath, "/")
		name = parts[len(parts)-1]
	}
	if c, ok := containerByWorker["_reviewer"]; ok {
		fmt.Fprintf(w, "👑 %s\treviewer\t%s\t%s\n", name, c.State, tdIssues["_reviewer"])
		delete(containerByWorker, "_reviewer")
	} else {
		fmt.Fprintf(w, "👑 %s\treviewer\tno container\t\n", name)
	}

	// Worker worktrees
	for wtName := range worktrees {
		if wtName == "main" {
			continue
		}
		task := tdIssues[wtName]
		if c, ok := containerByWorker[wtName]; ok {
			fmt.Fprintf(w, "🔨 %s\tworker\t%s\t%s\n", wtName, c.State, task)
			delete(containerByWorker, wtName)
		} else {
			fmt.Fprintf(w, "🔨 %s\tworker\tno container\t%s\n", wtName, task)
		}
	}

	// Orphaned containers (no worktree)
	for worker, c := range containerByWorker {
		fmt.Fprintf(w, "⚠  %s\torphan\t%s\t\n", worker, c.State)
	}

	w.Flush()
	return nil
}

func runWorkerStream(cmd *cobra.Command, args []string, raw bool) error {
	name := args[0]

	projectRoot, err := gitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	cName := "sindri-" + name
	if name == "reviewer" || name == "_reviewer" {
		cName = "sindri-reviewer"
	}

	// Verify container exists
	check := exec.Command("podman", "ps", "-a",
		"--filter", "label=sindri.project="+projectRoot,
		"--filter", "name="+cName,
		"--format", "{{.State}}")
	stateOut, err := check.Output()
	if err != nil || strings.TrimSpace(string(stateOut)) == "" {
		return fmt.Errorf("no container found for worker %q", name)
	}

	state := strings.TrimSpace(string(stateOut))
	if state != "running" {
		return fmt.Errorf("container %s is %s (not running)", cName, state)
	}

	fmt.Fprintf(os.Stderr, "Streaming %s (%s)...\n", name, cName)
	return streamWorkerLogs(cName, raw)
}

// streamWorkerLogs streams output from the Claude daemon inside a container.
func streamWorkerLogs(containerName string, raw bool) error {
	sessionID := getSessionUUID(containerName)
	if sessionID == "" {
		fmt.Fprintf(os.Stderr, "(no session found, falling back to podman logs)\n")
		logs := exec.Command("podman", "logs", "-f", containerName)
		logs.Stdout = os.Stdout
		logs.Stderr = os.Stderr
		return logs.Run()
	}
	fmt.Fprintf(os.Stderr, "(session: %s)\n", sessionID[:8])

	logs := exec.Command("podman", "exec", containerName, "claude", "logs", sessionID)

	if raw {
		logs.Stdout = os.Stdout
		logs.Stderr = os.Stderr
		return logs.Run()
	}

	// Beautified: parse stream-json lines, print readable output
	stdout, err := logs.StdoutPipe()
	if err != nil {
		return err
	}
	logs.Stderr = os.Stderr

	if err := logs.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()

		// Non-JSON lines (=== markers) pass through
		if !strings.HasPrefix(line, "{") {
			fmt.Println(line)
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			fmt.Println(line)
			continue
		}

		formatEvent(event)
	}

	return logs.Wait()
}

// ANSI color codes
const (
	dim    = "\033[2m"
	white  = "\033[1;37m"
	green  = "\033[32m"
	yellow = "\033[33m"
	reset  = "\033[0m"
)

func formatEvent(event map[string]interface{}) {
	typ, _ := event["type"].(string)

	switch typ {
	case "assistant":
		msg, ok := event["message"].(map[string]interface{})
		if !ok {
			return
		}
		content, ok := msg["content"].([]interface{})
		if !ok {
			return
		}
		for _, c := range content {
			block, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			blockType, _ := block["type"].(string)
			switch blockType {
			case "text":
				text, _ := block["text"].(string)
				fmt.Printf("\n  %s🤖💬 %s%s\n", white, text, reset)
			case "tool_use":
				name, _ := block["name"].(string)
				input, _ := block["input"].(map[string]interface{})
				switch name {
				case "Bash":
					cmd, _ := input["command"].(string)
					if len(cmd) > 120 {
						cmd = cmd[:120] + "..."
					}
					fmt.Printf("  %s🤖🔨 %s %s%s\n", dim, name, cmd, reset)
				case "Read", "Edit", "Write":
					path, _ := input["file_path"].(string)
					fmt.Printf("  %s🤖🔨 %s %s%s\n", dim, name, path, reset)
				default:
					fmt.Printf("  %s🤖🔨 %s%s\n", dim, name, reset)
				}
			}
		}

	case "user":
		toolResult, ok := event["tool_use_result"].(map[string]interface{})
		if !ok {
			return
		}
		if stdout, ok := toolResult["stdout"].(string); ok {
			out := stdout
			if len(out) > 200 {
				out = out[:200] + "..."
			}
			fmt.Printf("  %s   ← %s%s\n", dim, out, reset)
		}

	case "result":
		result, _ := event["result"].(string)
		cost, _ := event["total_cost_usd"].(float64)
		turns, _ := event["num_turns"].(float64)
		if len(result) > 200 {
			result = result[:200] + "..."
		}
		fmt.Printf("\n  %s▸ Done ($%.4f, %.0f turns)%s\n", green, cost, turns, reset)
		if result != "" {
			fmt.Printf("  %s🤖💬 %s%s\n\n", white, result, reset)
		}
	}
}

func runWorkerStart(cmd *cobra.Command, args []string, stream bool) error {
	name := args[0]

	projectRoot, err := gitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	// Check worktree exists
	wtPath := projectRoot + "/.worktrees/" + name
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		return fmt.Errorf("worktree %q not found at %s", name, wtPath)
	}

	// Ensure image is up to date
	if err := ensureImage(projectRoot); err != nil {
		return err
	}

	cName := "sindri-" + name

	// Remove stale container
	_ = exec.Command("podman", "rm", "-f", cName).Run()

	// Prepare claude home
	claudeHome := projectRoot + "/.worktrees/.claude-home-" + name
	_ = os.MkdirAll(claudeHome, 0755)
	homeDir, _ := os.UserHomeDir()
	credsSrc := homeDir + "/.claude/.credentials.json"
	if data, err := os.ReadFile(credsSrc); err == nil {
		_ = os.WriteFile(claudeHome+"/.credentials.json", data, 0600)
	}
	configPath := projectRoot + "/.worktrees/.claude.json"
	_ = os.WriteFile(configPath, []byte(`{"bypassPermissions":true,"bypassPermissionsModeAccepted":true,"hasCompletedOnboarding":true}`), 0644)

	// Write system prompt
	prompt := defaultWorkerPrompt
	_ = os.WriteFile(claudeHome+"/system-prompt.txt", []byte(prompt), 0644)

	image := "sindri-agent:test"

	podmanArgs := []string{
		"run", "-d",
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
		"-w", "/workspace",
		image,
		"sindri-agent",
	}

	fmt.Fprintf(os.Stderr, "Starting worker %s...\n", name)
	out, err := exec.Command("podman", podmanArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("podman run failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	fmt.Fprintf(os.Stderr, "Worker %s started.\n", name)
	if stream {
		return streamWorkerLogs(cName, false)
	}
	return nil
}

const defaultWorkerPrompt = `You are a Sindri worker agent. Your workspace is at /workspace (a git worktree). The main repo is at /repo (read-only).

IMPORTANT: All td commands must use -w /project flag, e.g. td -w /project next
IMPORTANT: Do NOT use EnterWorktree or create git worktrees. Work directly in /workspace.

WORKFLOW:
1. Run: TASK=$(wait-for-task) — this polls td for up to 5 minutes waiting for a task.
   If it exits non-zero, there is no work. Exit gracefully.
2. Run td -w /project start $TASK to claim it.
3. Read the task with td -w /project show $TASK.
4. If information is missing, ask via td -w /project comment $TASK "question" and td -w /project block $TASK --reason "waiting for answer", then wait with: wait-for-task
5. Implement the task. Run tests. Commit your changes.
6. Create a PR: gh pr create --title "summary" --body "details"
7. Record your work: td -w /project handoff $TASK --done "what you did"
8. Submit for review: td -w /project review $TASK
9. Wait for PR approval. Once approved: gh pr merge
10. Go back to step 1.

You cannot merge without human approval. Ask before guessing missing requirements.`

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
		return fmt.Errorf("failed to remove container: %s", strings.TrimSpace(string(out)))
	}
	fmt.Fprintf(os.Stderr, "Worker %s stopped.\n", name)
	return nil
}

func runWorkerInput(cmd *cobra.Command, args []string) error {
	name := args[0]
	message := args[1]

	projectRoot, err := gitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	cName := "sindri-" + name
	if name == "reviewer" || name == "_reviewer" {
		cName = "sindri-reviewer"
	}

	// Verify running
	check := exec.Command("podman", "ps",
		"--filter", "label=sindri.project="+projectRoot,
		"--filter", "name="+cName,
		"--format", "{{.State}}")
	stateOut, err := check.Output()
	if err != nil || strings.TrimSpace(string(stateOut)) != "running" {
		return fmt.Errorf("worker %q is not running", name)
	}

	fmt.Fprintf(os.Stderr, "→ %s: %s\n", name, message)

	// Get full session UUID from daemon
	sessionID := getSessionUUID(cName)

	var inputArgs []string
	if sessionID != "" {
		inputArgs = []string{"exec", cName, "claude", "--dangerously-skip-permissions",
			"--verbose", "--output-format", "stream-json",
			"--resume", sessionID, "-p", message}
	} else {
		inputArgs = []string{"exec", cName, "claude", "--dangerously-skip-permissions",
			"--verbose", "--output-format", "stream-json",
			"-c", "-p", message}
	}

	input := exec.Command("podman", inputArgs...)
	input.Stdout = os.Stdout
	input.Stderr = os.Stderr
	return input.Run()
}

func gitRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func containerName(c *podmanContainer) string {
	if len(c.Names) > 0 {
		return c.Names[0]
	}
	return "?"
}

// getTdInProgress queries td for in-progress issues and maps them to workers.
// Returns map[workerName] -> "td-xxx title"
func getTdInProgress(projectRoot string) map[string]string {
	result := make(map[string]string)
	out, err := exec.Command("td", "-w", projectRoot, "query", "status:in_progress").Output()
	if err != nil {
		return result
	}
	// Parse td query output: "td-abc123  [P2]  title  task  [in_progress]"
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		id := fields[0]
		// Extract title (everything between priority and type)
		title := ""
		for i := 2; i < len(fields); i++ {
			if fields[i] == "task" || fields[i] == "bug" || fields[i] == "feature" || fields[i] == "epic" || fields[i] == "chore" {
				break
			}
			if strings.HasPrefix(fields[i], "[") && strings.HasSuffix(fields[i], "]") {
				continue
			}
			if title != "" {
				title += " "
			}
			title += fields[i]
		}
		// For now, assign to any worker — td doesn't track which agent owns which task.
		// We use a simple heuristic: check td show for session info.
		result[id] = id + " " + title
	}

	// Flatten: we want map[workerName] -> task, but td doesn't track worker<->task.
	// Return as map[issueId] -> display for now, and also check sessions.
	// Simple approach: return all in-progress tasks, let the caller match.
	// For the list view, just show all in-progress tasks without worker mapping.
	flat := make(map[string]string)
	if len(result) > 0 {
		for _, display := range result {
			flat["_any"] = display
			break
		}
	}
	return flat
}

// ensureImage rebuilds sindri-agent:test if the Dockerfile changed or the build is stale.
// Build key = sha256(Dockerfile content + "YYYY-WW") so it rebuilds weekly or on change.
func ensureImage(projectRoot string) error {
	dockerfile := projectRoot + "/container/Dockerfile"
	content, err := os.ReadFile(dockerfile)
	if err != nil {
		// No Dockerfile in this repo — check if image exists already
		if exec.Command("podman", "image", "exists", "sindri-agent:test").Run() == nil {
			return nil
		}
		return fmt.Errorf("no Dockerfile at %s and no sindri-agent:test image found", dockerfile)
	}

	year, week := time.Now().ISOWeek()
	timeKey := fmt.Sprintf("%d-%d", year, week)
	h := sha256.New()
	h.Write(content)
	h.Write([]byte(timeKey))
	buildKey := fmt.Sprintf("%x", h.Sum(nil))[:16]

	// Check cached build key
	cacheFile := projectRoot + "/.worktrees/.build-key"
	if cached, err := os.ReadFile(cacheFile); err == nil && strings.TrimSpace(string(cached)) == buildKey {
		return nil // up to date
	}

	fmt.Fprintf(os.Stderr, "Building container image...\n")

	// Stage host binaries
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

// getSessionUUID gets the full session UUID from the claude daemon inside a container.
func getSessionUUID(containerName string) string {
	out, err := exec.Command("podman", "exec", containerName,
		"claude", "agents", "--json").Output()
	if err != nil {
		return ""
	}
	var agents []map[string]interface{}
	if err := json.Unmarshal(out, &agents); err != nil || len(agents) == 0 {
		return ""
	}
	sid, _ := agents[0]["sessionId"].(string)
	return sid
}

func parseWorktreeNames(output, mainDir string) map[string]string {
	names := make(map[string]string)
	var currentPath string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
			name := "main"
			if currentPath != mainDir {
				parts := strings.Split(currentPath, "/")
				name = parts[len(parts)-1]
			}
			names[name] = currentPath
		}
	}
	return names
}
