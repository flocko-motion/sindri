// package: tui / util
// type:    ui
// job:     the selector row type and tiny generic helpers used across the
// tab/component files.
// limits:  tiny generic helpers only; no domain logic and no rendering of its
// own (-> the tabs/components).
package tui

import (
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/api"
)

// repoName maps a project's repoTag to its short repo name, over the board's registry.
func (m model) repoName(tag string) string { return api.RepoName(m.state.Projects, tag) }

// repoPath maps a project's repoTag to its absolute path, or "" when the board's registry has
// no such project.
func (m model) repoPath(tag string) string {
	for _, p := range m.state.Projects {
		if p.Tag == tag {
			return p.Path
		}
	}
	return ""
}

// taskTitle maps a task id to its cached title, or "" — a task from another project (the board
// only carries the selected one), or one closed/scrapped since the cache was built.
func (m model) taskTitle(id string) string {
	for _, t := range m.state.Tasks {
		if t.ID == id {
			return t.Title
		}
	}
	return ""
}

// taskLabel is a task id with its title alongside it ("id  title") when known, else the bare id,
// else "-". Used where a task shows only as an id today, leaving no clue what it actually is.
func (m model) taskLabel(id string) string {
	if id == "" {
		return dash(id)
	}
	if title := m.taskTitle(id); title != "" {
		return id + "  " + title
	}
	return id
}

// prTask is PR id's underlying task, or "" — a reviewer's AgentView carries no task of its own
// (the task belongs to the agent that wrote the PR), so this is how the Agents tab still says
// what a reviewer's PR is for.
func (m model) prTask(id string) string {
	if id == "" {
		return ""
	}
	for _, p := range m.state.PRs {
		if p.ID == id {
			return p.Task
		}
	}
	return ""
}

// agentWorkspacePath is an agent's workspace as an ABSOLUTE path, or "". Workspace is
// repo-relative, so it is joined to the agent's OWN project — the Agents tab can show a fleet
// spanning repos — and absolute because it becomes a child process's working directory.
func (m model) agentWorkspacePath(name string) string {
	for _, a := range m.state.Agents {
		if a.Name != name {
			continue
		}
		root := m.repoPath(a.Project)
		if root == "" || a.Workspace == "" {
			return ""
		}
		return filepath.Join(root, a.Workspace)
	}
	return ""
}

// selWorktree is the tree the selected row stands for: an agent's own, or on the PRs tab its
// author's. Empty once that agent is gone — a PR outlives the tree behind it.
func (m model) selWorktree() string {
	switch m.tab {
	case 1:
		return m.agentWorkspacePath(m.selID())
	case 2:
		for _, p := range m.state.PRs {
			if p.ID == m.selID() {
				return m.agentWorkspacePath(p.Agent)
			}
		}
	}
	return ""
}

// agentOnTask is the agent working task id, and whether one is. It matches a package too: a
// hierarchy is claimed whole and the agent's state names the SUBTASK, so walking up from each
// agent's task is what makes the epic — and everything under it — reach the agent holding it.
func (m model) agentOnTask(id string) (api.AgentView, bool) {
	if id == "" {
		return api.AgentView{}, false
	}
	parent := make(map[string]string, len(m.state.Tasks))
	for _, t := range m.state.Tasks {
		parent[t.ID] = t.ParentID
	}
	for _, a := range m.state.Agents {
		if a.Task == "" {
			continue
		}
		// Up to 32 levels: deep enough for any real hierarchy, and a hard stop should the
		// cached parent links ever form a cycle.
		for cur, depth := a.Task, 0; cur != "" && depth < 32; cur, depth = parent[cur], depth+1 {
			if cur == id {
				return a, true
			}
		}
	}
	return api.AgentView{}, false
}

// agentOnPR is the agent that authored PR id, and whether one is — the PR's own Agent field,
// resolved to its live AgentView so attach gets status and container, not just a name.
func (m model) agentOnPR(id string) (api.AgentView, bool) {
	if id == "" {
		return api.AgentView{}, false
	}
	for _, p := range m.state.PRs {
		if p.ID != id {
			continue
		}
		for _, a := range m.state.Agents {
			if a.Name == p.Agent {
				return a, true
			}
		}
	}
	return api.AgentView{}, false
}

// repoColorIdx is a repo's pinned colour choice from the registry (0 = default).
func (m model) repoColorIdx(tag string) int {
	for _, p := range m.state.Projects {
		if p.Tag == tag {
			return p.Color
		}
	}
	return 0
}

// repoStyle colours text in a repo's bright shade, honouring its pinned colour.
func (m model) repoStyle(tag string) lipgloss.Style {
	return repoStyleFor(tag, m.repoColorIdx(tag))
}

// currentRepo returns the selected repo's short name and tag (resolved from m.root),
// or ("","") when nothing matches — the source for the persistent header indicator.
func (m model) currentRepo() (name, tag string) {
	for _, p := range m.state.Projects {
		if p.Path == m.root {
			return filepath.Base(p.Path), p.Tag
		}
	}
	return "", ""
}

// row is one selector line: display text + the id it selects ("" = not selectable).
type row struct {
	text string
	id   string
}

func rowTexts(rows []row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.text
	}
	return out
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
