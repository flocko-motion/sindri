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

	// Query td for in-progress tasks
	tasks := getInProgressTasks(projectRoot)
	taskIdx := 0
	for i := range workers {
		if workers[i].Role == "worker" && workers[i].Status == "running" && taskIdx < len(tasks) {
			workers[i].Task = tasks[taskIdx]
			taskIdx++
		}
	}

	// Match PRs to workers by branch name (pr-<name> → worker <name>)
	prs, _ := store.ListFor(projectRoot)
	for _, pr := range prs {
		if pr.Status == "merged" {
			continue
		}
		for i := range workers {
			if workers[i].IsMain {
				continue
			}
			if pr.Branch == workers[i].Name {
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

func getInProgressTasks(projectRoot string) []string {
	out, err := exec.Command("td", "-w", projectRoot, "query", "status:in_progress").Output()
	if err != nil {
		return nil
	}
	var tasks []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "td-") {
			continue
		}
		// "td-abc123  [P2]  Title  task  [in_progress]" → "td-abc123 Title"
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		id := fields[0]
		var title []string
		for _, f := range fields[2:] {
			if f == "task" || f == "bug" || f == "feature" || f == "chore" || f == "epic" {
				break
			}
			if strings.HasPrefix(f, "[") && strings.HasSuffix(f, "]") {
				continue
			}
			title = append(title, f)
		}
		tasks = append(tasks, id+" "+strings.Join(title, " "))
	}
	return tasks
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
