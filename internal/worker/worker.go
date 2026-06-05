// package: worker / worker
// type:    adapter (podman + git worktrees)
// job:     discovery/status — lists workers by joining git worktrees, podman
//          containers, and each worktree's branch/task.
// limits:  start/stop/create live in lifecycle.go (neighbour); task data comes
//          from adapter/td, PRs from ghlocal/store.
package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/flo-at/sindri/internal/sindri"
)

type Worker struct {
	Name      string
	Role      string // "worker" or "reviewer"
	Status    string // "running", "exited", or "-"
	IsMain    bool
	Path      string // worktree path
	Container string // container name or ""
	Task      string // current td task (e.g. "td-abc123 Add greeting module")
	PR        string // open PR status (e.g. "pr-fjalar [open]")
	Branch    string // current git branch in the worktree
}

type podmanContainer struct {
	Names  []string          `json:"Names"`
	State  string            `json:"State"`
	Labels map[string]string `json:"Labels"`
}

// List reconciles the agent index (the roll call of agents that should exist)
// against observed reality (podman containers + git worktrees). The index is
// the spine: agents come from it, with live state attached. Containers or
// worktrees with no index entry surface as orphans.
func List(projectRoot string) []Worker {
	// Roll call: the index is the canonical set of agents.
	roster, err := sindri.Roster(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: read agent index: %v\n", err)
	}

	// Attendance: podman containers keyed by their sindri.worker label.
	containerByName := make(map[string]*podmanContainer)
	out, cerr := exec.Command("podman", "ps", "-a",
		"--filter", "label=sindri.project="+projectRoot,
		"--format", "json",
	).Output()
	if cerr == nil && len(strings.TrimSpace(string(out))) > 0 {
		var containers []podmanContainer
		if json.Unmarshal(out, &containers) == nil {
			for i, c := range containers {
				if w := c.Labels["sindri.worker"]; w != "" {
					containerByName[w] = &containers[i]
				}
			}
		}
	}

	// Worktrees on disk, keyed by name (excludes the main repo).
	wtOut, _ := exec.Command("git", "-C", projectRoot, "worktree", "list", "--porcelain").Output()
	worktrees := parseWorktreeNames(string(wtOut), projectRoot)

	var workers []Worker
	seen := make(map[string]bool)

	// 1. Indexed agents — role and identity come from the index, never position.
	for _, a := range roster {
		seen[a.Name] = true
		w := Worker{
			Name:   a.Name,
			Role:   a.Role,
			IsMain: a.Role == "reviewer",
		}
		if a.Workspace != "" {
			w.Path = filepath.Join(projectRoot, a.Workspace)
		}

		var running bool
		if c, ok := containerByName[a.Name]; ok {
			w.Container = containerName(c)
			running = c.State == "running"
			delete(containerByName, a.Name)
		}

		workspaceMissing := a.Workspace != "" && !dirExists(w.Path)
		w.Branch = worktreeBranch(projectRoot, a.Name)
		w.Task = taskFor(w.Path, w.Branch)
		w.Status = reconcileStatus(running, workspaceMissing, w.Task != "")
		workers = append(workers, w)
	}

	// 2. Worktrees with no index entry → orphans (stale worktree).
	for name, path := range worktrees {
		if name == "main" || seen[name] {
			continue
		}
		seen[name] = true
		w := Worker{Name: name, Role: "orphan", Path: path, Status: "-"}
		if c, ok := containerByName[name]; ok {
			w.Container = containerName(c)
			w.Status = c.State
			delete(containerByName, name)
		}
		w.Branch = worktreeBranch(projectRoot, name)
		workers = append(workers, w)
	}

	// 3. Containers with no index entry → orphans (stale pod).
	for name, c := range containerByName {
		workers = append(workers, Worker{
			Name:      name,
			Role:      "orphan",
			Status:    c.State,
			Container: containerName(c),
		})
	}

	attachTasks(projectRoot, workers)
	attachPRs(projectRoot, workers)
	return workers
}

// Orphans returns the agents that have a container or worktree but no index
// entry — most likely stale, and safe to prune after confirmation.
func Orphans(projectRoot string) []Worker {
	var orphans []Worker
	for _, w := range List(projectRoot) {
		if w.Role == "orphan" {
			orphans = append(orphans, w)
		}
	}
	return orphans
}

// RemoveOrphan deletes an orphan's container and worktree.
func RemoveOrphan(projectRoot string, w Worker) error {
	if w.Container != "" {
		if out, err := exec.Command("podman", "rm", "-f", w.Container).CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: podman rm -f %s: %s\n", w.Container, strings.TrimSpace(string(out)))
		}
	}
	if w.Path != "" {
		if out, err := exec.Command("git", "-C", projectRoot, "worktree", "remove", "--force", w.Path).CombinedOutput(); err != nil {
			// Fall back to a manual removal + prune if git refuses.
			if rmErr := os.RemoveAll(w.Path); rmErr != nil {
				return fmt.Errorf("remove worktree %s: %s", w.Path, strings.TrimSpace(string(out)))
			}
			_ = exec.Command("git", "-C", projectRoot, "worktree", "prune").Run()
		}
	}
	return nil
}

// containerName returns a container's first name, or "".
func containerName(c *podmanContainer) string {
	if len(c.Names) > 0 {
		return c.Names[0]
	}
	return ""
}

func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// reconcileStatus turns (declared vs observed) into an actionable status.
func reconcileStatus(running, workspaceMissing, hasTask bool) string {
	switch {
	case running:
		return "running"
	case workspaceMissing:
		return "no-workspace"
	case hasTask:
		return "crashed"
	default:
		return "idle"
	}
}

// taskFor reads an agent's current task ID from its workspace .sindri-task file,
// falling back to a td-… branch name.
func taskFor(workspacePath, branch string) string {
	if workspacePath != "" {
		if data, err := os.ReadFile(filepath.Join(workspacePath, ".sindri-task")); err == nil {
			if id := strings.TrimSpace(string(data)); id != "" {
				return id
			}
		}
	}
	if strings.HasPrefix(branch, "td-") {
		return branch
	}
	return ""
}

// attachTasks expands each worker's bare task ID into "td-xxx Title".
func attachTasks(projectRoot string, workers []Worker) {
	taskTitles := getTaskTitles(projectRoot)
	for i := range workers {
		id := workers[i].Task
		if id == "" {
			continue
		}
		if title, ok := taskTitles[id]; ok {
			workers[i].Task = id + " " + title
		}
	}
}

// attachPRs matches each worker's task to an open PR.
func attachPRs(projectRoot string, workers []Worker) {
	prs, _ := store.ListFor(projectRoot)
	for i := range workers {
		if workers[i].Task == "" {
			continue
		}
		taskID := strings.Fields(workers[i].Task)[0]
		for _, pr := range prs {
			if pr.Status == "merged" {
				continue
			}
			if pr.Branch == taskID {
				workers[i].PR = pr.ID + " [" + pr.Status + "]"
				break
			}
		}
	}
}

func getTaskTitles(projectRoot string) map[string]string {
	out, err := exec.Command("td", "-w", projectRoot, "list", "--json", "--limit", "50").Output()
	if err != nil {
		return nil
	}
	var tasks []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if json.Unmarshal(out, &tasks) != nil {
		return nil
	}
	m := make(map[string]string, len(tasks))
	for _, t := range tasks {
		m[t.ID] = t.Title
	}
	return m
}

// worktreeBranch reads the branch from .git/worktrees/<name>/HEAD in the main repo.
// This works even while the container is running (the worktree .git file has container paths).
func worktreeBranch(projectRoot, name string) string {
	headPath := filepath.Join(projectRoot, ".git", "worktrees", name, "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	ref := strings.TrimSpace(string(data))
	if strings.HasPrefix(ref, "ref: refs/heads/") {
		return strings.TrimPrefix(ref, "ref: refs/heads/")
	}
	if len(ref) >= 8 {
		return ref[:8]
	}
	return ref
}

func parseWorktreeNames(output, mainDir string) map[string]string {
	wtDir := mainDir + "/.worktrees/"
	names := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			path := strings.TrimPrefix(line, "worktree ")
			if path == mainDir {
				names["main"] = path
			} else if strings.HasPrefix(path, wtDir) {
				name := strings.TrimPrefix(path, wtDir)
				names[name] = path
			}
			// Ignore worktrees outside .worktrees/ (e.g. container paths)
		}
	}
	return names
}
