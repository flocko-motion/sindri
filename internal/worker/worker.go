package worker

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/flo-at/sindri/internal/ghlocal/store"
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

// List returns all workers for a project: worktrees + container status.
func List(projectRoot string) []Worker {
	// Query podman containers
	containerByWorker := make(map[string]*podmanContainer)
	out, err := exec.Command("podman", "ps", "-a",
		"--filter", "label=sindri.project="+projectRoot,
		"--format", "json",
	).Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		var containers []podmanContainer
		if json.Unmarshal(out, &containers) == nil {
			for i, c := range containers {
				if w := c.Labels["sindri.worker"]; w != "" {
					containerByWorker[w] = &containers[i]
				}
			}
		}
	}

	// Query git worktrees
	wtOut, _ := exec.Command("git", "-C", projectRoot, "worktree", "list", "--porcelain").Output()
	worktrees := parseWorktreeNames(string(wtOut), projectRoot)

	var workers []Worker

	// Main repo = reviewer
	mainName := "sindri"
	if p, ok := worktrees["main"]; ok {
		parts := strings.Split(p, "/")
		mainName = parts[len(parts)-1]
	}
	status := "-"
	cName := ""
	if c, ok := containerByWorker["_reviewer"]; ok {
		status = c.State
		if len(c.Names) > 0 {
			cName = c.Names[0]
		}
		delete(containerByWorker, "_reviewer")
	}
	workers = append(workers, Worker{
		Name:      mainName,
		Role:      "reviewer",
		Status:    status,
		IsMain:    true,
		Path:      worktrees["main"],
		Container: cName,
	})

	// Worker worktrees
	for name, path := range worktrees {
		if name == "main" || name == "review" {
			continue
		}
		status := "-"
		cName := ""
		if c, ok := containerByWorker[name]; ok {
			status = c.State
			if len(c.Names) > 0 {
				cName = c.Names[0]
			}
			delete(containerByWorker, name)
		}
		workers = append(workers, Worker{
			Name:      name,
			Role:      "worker",
			Status:    status,
			Path:      path,
			Container: cName,
		})
	}

	// Orphaned containers
	for name, c := range containerByWorker {
		cName := ""
		if len(c.Names) > 0 {
			cName = c.Names[0]
		}
		workers = append(workers, Worker{
			Name:      name,
			Role:      "orphan",
			Status:    c.State,
			Container: cName,
		})
	}

	// Read task from .sindri-task state file in each worktree
	taskTitles := getTaskTitles(projectRoot)
	for i := range workers {
		if workers[i].Path == "" {
			continue
		}
		taskFile := filepath.Join(workers[i].Path, ".sindri-task")
		if data, err := os.ReadFile(taskFile); err == nil {
			taskID := strings.TrimSpace(string(data))
			if taskID != "" {
				if title, ok := taskTitles[taskID]; ok {
					workers[i].Task = taskID + " " + title
				} else {
					workers[i].Task = taskID
				}
			}
		}
	}

	// Match PRs to workers by task ID
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

	// Read current branch for each worker's worktree
	for i := range workers {
		if workers[i].Path != "" {
			workers[i].Branch = worktreeBranch(workers[i].Path)
		}
	}

	return workers
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

func worktreeBranch(worktreePath string) string {
	headPath := filepath.Join(worktreePath, ".git")
	info, err := os.Stat(headPath)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		headPath = filepath.Join(headPath, "HEAD")
	} else {
		data, err := os.ReadFile(headPath)
		if err != nil {
			return ""
		}
		line := strings.TrimSpace(string(data))
		if !strings.HasPrefix(line, "gitdir: ") {
			return ""
		}
		gitdir := strings.TrimPrefix(line, "gitdir: ")
		if !filepath.IsAbs(gitdir) {
			gitdir = filepath.Join(worktreePath, gitdir)
		}
		headPath = filepath.Join(gitdir, "HEAD")
	}
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
