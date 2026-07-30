// package: tui / util
// type:    small shared helpers
// job:     the selector row type and tiny generic helpers used across the
// tab/component files.
// limits:  tiny generic helpers only; no domain logic and no rendering of its
// own (-> the tabs/components).
package tui

import (
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/hub"
)

// repoName maps a project's repoTag to its short repo name (its path's basename),
// falling back to the tag when the project isn't in the board's registry.
func (m model) repoName(tag string) string {
	for _, p := range m.state.Projects {
		if p.Tag == tag {
			return filepath.Base(p.Path)
		}
	}
	return tag
}

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

// agentWorkspacePath is an agent's workspace as an ABSOLUTE path, or "".
//
// AgentView.Workspace is repo-relative (".worktrees/<name>", or "." for a coauthor sharing the
// checkout), so it only names a real directory alongside its own repo — and it is resolved
// against the agent's OWN project, not the selected one, since the Agents tab can show a fleet
// spanning several repos. A relative path handed to a child process would instead resolve
// against wherever the TUI happens to have been launched.
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

// agentOnTask is the agent working task id, and whether one is.
//
// It matches a package too, not just the exact row: a hierarchy is claimed whole and the agent's
// state names the SUBTASK it is on, so selecting the parent — the row that represents the work —
// would otherwise find nobody. Walking up from each agent's task means the epic and every task
// beneath it all lead to the one agent holding that tree.
func (m model) agentOnTask(id string) (hub.AgentView, bool) {
	if id == "" {
		return hub.AgentView{}, false
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
	return hub.AgentView{}, false
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
