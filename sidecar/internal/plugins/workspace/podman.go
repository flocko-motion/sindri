package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// launchSindriContainer starts a podman container for a worker.
func launchSindriContainer(projectRoot string, wt *Worktree) tea.Cmd {
	return func() tea.Msg {
		image := "sindri-agent:latest"
		containerName := "sindri-" + wt.Name
		claudeHome := filepath.Join(projectRoot, ".worktrees", ".claude-home-"+wt.Name)

		// Prepare claude home with credentials
		if err := os.MkdirAll(claudeHome, 0755); err != nil {
			return SindriLaunchMsg{WorkerName: wt.Name, Err: err}
		}
		homeDir, _ := os.UserHomeDir()
		credsSrc := filepath.Join(homeDir, ".claude", ".credentials.json")
		if data, err := os.ReadFile(credsSrc); err == nil {
			_ = os.WriteFile(filepath.Join(claudeHome, ".credentials.json"), data, 0600)
		}
		// Minimal claude config
		configPath := filepath.Join(claudeHome, "..", ".claude.json")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			_ = os.WriteFile(configPath, []byte("{}"), 0644)
		}

		args := []string{
			"run", "-d",
			"--name", containerName,
			"--userns=keep-id",
			"--label", "sindri.project=" + projectRoot,
			"--label", "sindri.worker=" + wt.Name,
			"-v", wt.Path + ":/workspace:rw,z",
			"-v", projectRoot + ":/repo:ro,z",
			"-v", claudeHome + ":/home/sindri/.claude:rw,z",
			"-v", configPath + ":/home/sindri/.claude.json:rw,z",
			"-e", "GH_LOCAL_BASE=main",
			"-e", "TD_ROOT=" + projectRoot + "/.todos",
			"-w", "/workspace",
			image,
			"claude", "--dangerously-skip-permissions",
		}

		cmd := exec.Command("podman", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return SindriLaunchMsg{WorkerName: wt.Name, Err: fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)}
		}
		return SindriLaunchMsg{WorkerName: wt.Name}
	}
}

// SindriLaunchMsg is sent when a podman container launch completes.
type SindriLaunchMsg struct {
	WorkerName string
	Err        error
}

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
