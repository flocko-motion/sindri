package workspace

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// PodmanContainer represents a running Sindri container.
type PodmanContainer struct {
	Name    string
	Worker  string // sindri.worker label
	Project string // sindri.project label
	Status  string // e.g. "running", "exited"
	Created time.Time
}

// podmanInspect is the subset of `podman ps --format json` we care about.
type podmanInspect struct {
	Names   []string          `json:"Names"`
	State   string            `json:"State"`
	Labels  map[string]string `json:"Labels"`
	Created string            `json:"Created"`
}

const defaultWorkerPrompt = `You are a Sindri worker agent. Your workspace is at /workspace (a git worktree). The main repo is at /repo (read-only).

WORKFLOW:
1. Run: TASK=$(wait-for-task) — this polls td for up to 5 minutes waiting for a task.
   If it exits non-zero, there is no work. Exit gracefully.
2. Run td start $TASK to claim it.
3. Read the task with td show $TASK.
4. If information is missing, ask via td comment $TASK "question" and td block $TASK --reason "waiting for answer", then wait with: wait-for-task
5. Implement the task. Run tests. Commit your changes.
6. Create a PR: gh pr create --title "summary" --body "details"
7. Record your work: td handoff $TASK --done "what you did"
8. Submit for review: td review $TASK
9. Wait for PR approval. Once approved: gh pr merge
10. Go back to step 1.

You cannot merge without human approval. Ask before guessing missing requirements.`

const defaultReviewerPrompt = `You are a Sindri reviewer agent. Your workspace is at /workspace (the main repo, read-write). Worker worktrees are mounted at /worktrees/ (read-only).

WORKFLOW:
1. Check for PRs awaiting review: gh pr list
2. For each open PR: review the diff with gh pr view <id>
3. Check the worker's worktree code at /worktrees/<name>/
4. If changes look good: gh pr review <id> --approve
5. If you have feedback: td comment <task-id> "your feedback"
6. Periodically check td status for blocked tasks that need answers.
7. Answer questions via td comment <id> "answer" and td unblock <id>.

You are the quality gate. Be thorough but constructive.`

// sindriImage returns the container image to use.
// Checks for sindri-agent:latest first, falls back to sindri-agent:test.
func sindriImage() string {
	if exec.Command("podman", "image", "exists", "sindri-agent:latest").Run() == nil {
		return "sindri-agent:latest"
	}
	return "sindri-agent:test"
}

// podmanPollTickMsg triggers the next podman status poll.
type podmanPollTickMsg struct{}

// PodmanStatusMsg carries the result of a podman query.
type PodmanStatusMsg struct {
	Containers []PodmanContainer
	Err        error
}

// pollPodmanStatus queries podman for running Sindri containers for this project.
func pollPodmanStatus(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		containers, err := queryPodmanContainers(projectRoot)
		return PodmanStatusMsg{Containers: containers, Err: err}
	}
}

// queryPodmanContainers lists all sindri containers for the given project.
func queryPodmanContainers(projectRoot string) ([]PodmanContainer, error) {
	cmd := exec.Command("podman", "ps", "-a",
		"--filter", "label=sindri.project="+projectRoot,
		"--format", "json",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	if len(strings.TrimSpace(string(out))) == 0 {
		return nil, nil
	}

	var raw []podmanInspect
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, err
	}

	var containers []PodmanContainer
	for _, r := range raw {
		name := ""
		if len(r.Names) > 0 {
			name = r.Names[0]
		}
		containers = append(containers, PodmanContainer{
			Name:    name,
			Worker:  r.Labels["sindri.worker"],
			Project: r.Labels["sindri.project"],
			Status:  r.State,
		})
	}
	return containers, nil
}

// workerHasContainer checks if a worker name has a running container.
func workerHasContainer(containers []PodmanContainer, workerName string) (bool, string) {
	for _, c := range containers {
		if c.Worker == workerName {
			return true, c.Status
		}
	}
	return false, ""
}

// launchSindriContainer starts a podman container for a worker or reviewer.
func launchSindriContainer(projectRoot string, wt *Worktree) tea.Cmd {
	return func() tea.Msg {
		image := sindriImage()

		workerLabel := wt.Name
		containerName := "sindri-" + wt.Name
		if wt.IsMain {
			workerLabel = "_reviewer"
			containerName = "sindri-reviewer"
		}

		claudeHome := filepath.Join(projectRoot, ".worktrees", ".claude-home-"+workerLabel)
		if err := os.MkdirAll(claudeHome, 0755); err != nil {
			return SindriLaunchMsg{WorkerName: wt.Name, Err: err}
		}
		homeDir, _ := os.UserHomeDir()
		credsSrc := filepath.Join(homeDir, ".claude", ".credentials.json")
		if data, err := os.ReadFile(credsSrc); err == nil {
			_ = os.WriteFile(filepath.Join(claudeHome, ".credentials.json"), data, 0600)
		}
		configPath := filepath.Join(claudeHome, "..", ".claude.json")
		_ = os.WriteFile(configPath, []byte(`{"bypassPermissions":true}`), 0644)

		// Remove any stale container with the same name
		_ = exec.Command("podman", "rm", "-f", containerName).Run()

		args := []string{
			"run", "-d",
			"--name", containerName,
			"--userns=keep-id",
			"--label", "sindri.project=" + projectRoot,
			"--label", "sindri.worker=" + workerLabel,
			"-v", claudeHome + ":/home/sindri/.claude:rw,z",
			"-v", configPath + ":/home/sindri/.claude.json:rw,z",
			"-e", "GH_LOCAL_BASE=main",
			"-e", "TD_ROOT=" + projectRoot + "/.todos",
		}

		if wt.IsMain {
			// Reviewer: rw on main repo, ro on all worktrees
			args = append(args,
				"-v", projectRoot+":/workspace:rw,z",
				"-v", filepath.Join(projectRoot, ".worktrees")+":/worktrees:ro,z",
			)
		} else {
			// Worker: rw on worktree, ro on main repo
			args = append(args,
				"-v", wt.Path+":/workspace:rw,z",
				"-v", projectRoot+":/repo:ro,z",
			)
		}

		prompt := defaultWorkerPrompt
		if wt.IsMain {
			prompt = defaultReviewerPrompt
		}

		// Write system prompt for the agent script
		promptFile := filepath.Join(claudeHome, "system-prompt.txt")
		_ = os.WriteFile(promptFile, []byte(prompt), 0644)

		// Run the sindri-agent wrapper which starts claude --bg and stays alive
		args = append(args,
			"-w", "/workspace",
			image,
			"sindri-agent",
		)

		cmd := exec.Command("podman", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return SindriLaunchMsg{WorkerName: wt.Name, Err: fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)}
		}
		// Container started — return immediately, log stream will detect failures
		return SindriLaunchMsg{WorkerName: wt.Name}
	}
}

// SindriLaunchMsg is sent when a podman container launch completes.
type SindriLaunchMsg struct {
	WorkerName string
	Err        error
}

// PodmanLogsMsg carries a new chunk of log output.
type PodmanLogsMsg struct {
	WorkerName string
	Lines      string
}

// PodmanLogStreamDoneMsg signals the log stream ended.
type PodmanLogStreamDoneMsg struct {
	WorkerName string
}

// startLogStream starts streaming output from the Claude background session
// inside a container via `podman exec <container> claude logs <session>`.
// Falls back to `podman logs -f` if claude logs isn't available yet.
func startLogStream(workerName string, isMain bool) tea.Cmd {
	containerName := containerNameFor(workerName, isMain)

	return func() tea.Msg {
		// Try podman logs -f first (captures startup output including --bg session ID)
		cmd := exec.Command("podman", "logs", "-f", containerName)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return PodmanLogStreamDoneMsg{WorkerName: workerName}
		}
		cmd.Stderr = cmd.Stdout

		if err := cmd.Start(); err != nil {
			return PodmanLogStreamDoneMsg{WorkerName: workerName}
		}

		buf := make([]byte, 4096)
		n, readErr := stdout.Read(buf)
		if n > 0 {
			activeLogStreams.Store(workerName, &logStream{cmd: cmd, reader: stdout})
			return PodmanLogsMsg{WorkerName: workerName, Lines: string(buf[:n])}
		}
		_ = cmd.Wait()
		if readErr != nil {
			return PodmanLogStreamDoneMsg{WorkerName: workerName}
		}
		return PodmanLogStreamDoneMsg{WorkerName: workerName}
	}
}

// waitForLogChunk reads the next chunk from an active log stream.
func waitForLogChunk(workerName string) tea.Cmd {
	return func() tea.Msg {
		val, ok := activeLogStreams.Load(workerName)
		if !ok {
			return PodmanLogStreamDoneMsg{WorkerName: workerName}
		}
		stream := val.(*logStream)
		buf := make([]byte, 4096)
		n, _ := stream.reader.Read(buf)
		if n > 0 {
			return PodmanLogsMsg{WorkerName: workerName, Lines: string(buf[:n])}
		}
		activeLogStreams.Delete(workerName)
		_ = stream.cmd.Wait()
		return PodmanLogStreamDoneMsg{WorkerName: workerName}
	}
}

// containerNameFor returns the podman container name for a worker.
func containerNameFor(workerName string, isMain bool) string {
	if isMain {
		return "sindri-reviewer"
	}
	return "sindri-" + workerName
}

type logStream struct {
	cmd    *exec.Cmd
	reader io.Reader
}

var activeLogStreams sync.Map

// orphanedContainers returns containers that don't match any worktree.
func orphanedContainers(containers []PodmanContainer, worktrees []*Worktree) []PodmanContainer {
	wtNames := make(map[string]bool)
	for _, wt := range worktrees {
		wtNames[wt.Name] = true
	}
	var orphans []PodmanContainer
	for _, c := range containers {
		if c.Worker != "" && !wtNames[c.Worker] {
			orphans = append(orphans, c)
		}
	}
	return orphans
}
