// package: tui / repos tab
// type:    ui (the Repos tab — the registry surface)
// job:     list the repos the hub tracks (the TUI counterpart of `sindri repo …`):
//          each in its colour, with agent count and live/current markers; enter
//          switches to a repo, D forgets it (agent-guarded, files kept). Detail shows
//          the repo's path, agents, and PRs.
// limits:  rendering + action wiring; the registry lives in the hub (repo.go).
package tui

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// repoRows lists the registered repos (switcher order: live-agents-first → recency →
// alphabetical), each name in its repo colour with an agent count and markers. The
// row id is the repo tag.
func (m model) repoRows() []row {
	var out []row
	for _, p := range m.switcherOrder() {
		label := m.repoName(p.Tag)
		if m.repoHasLiveAgent(p.Tag) {
			label += " ●"
		}
		if p.Path == m.root {
			label += " ✓"
		}
		if pad := 20 - lipgloss.Width(label); pad > 0 {
			label += repeat(pad)
		}
		text := m.repoStyle(p.Tag).Render(label) + fmt.Sprintf("  %2d agents  %s", m.repoAgentCount(p.Tag), p.Path)
		out = append(out, row{text, p.Tag})
	}
	return out
}

// repeat is n spaces (a tiny helper so repoRows reads cleanly).
func repeat(n int) string {
	s := make([]byte, n)
	for i := range s {
		s[i] = ' '
	}
	return string(s)
}

// repoAgentCount is how many agents the repo has on its roster (from the board).
func (m model) repoAgentCount(tag string) int {
	n := 0
	for _, a := range m.state.Agents {
		if a.Project == tag {
			n++
		}
	}
	return n
}

// repoDetailLines renders the selected repo's detail: path, tag, its agents, and its
// PRs — all from the board snapshot (no fetch).
func (m model) repoDetailLines() []string {
	tag := m.selID()
	if tag == "" {
		return []string{dimStyle.Render("(no repos)")}
	}
	var path, last string
	for _, p := range m.state.Projects {
		if p.Tag == tag {
			path, last = p.Path, p.LastUsed
		}
	}
	ls := []string{
		"repo:   " + m.repoName(tag),
		"path:   " + path,
		"tag:    " + tag,
	}
	if last != "" {
		ls = append(ls, "used:   "+shortAge(last)+" ago")
	}
	var agents, prs []string
	for _, a := range m.state.Agents {
		if a.Project == tag {
			agents = append(agents, a.Name+" ("+a.Status+")")
		}
	}
	for _, p := range m.state.PRs {
		if p.Project == tag && p.Status != "merged" {
			prs = append(prs, p.ID+" "+p.Status)
		}
	}
	ls = append(ls, "", "agents: "+joinOrDash(agents))
	ls = append(ls, "open prs: "+joinOrDash(prs))
	ls = append(ls, "", dimStyle.Render("enter switch · E config · D forget"))
	return ls
}

// joinOrDash renders a "-" for an empty list, else the entries one per line-ready
// comma join (kept short — the detail pane is narrow).
func joinOrDash(xs []string) string {
	if len(xs) == 0 {
		return "-"
	}
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}

// openColorChoice opens a picker of colour swatches for a repo: "default" (the
// hash-derived hue) plus each palette choice rendered in its own bright shade, so the
// list is visual. Selecting one pins it in the registry (per-machine display pref).
func (m *model) openColorChoice(tag string) {
	cl := m.cl
	opts := []string{"default (auto)"}
	vals := []string{"0"}
	for i := 1; i <= nRepoColors; i++ {
		swatch := repoStyleFor(tag, i).Render("████")
		opts = append(opts, fmt.Sprintf("%s  colour %d", swatch, i))
		vals = append(vals, strconv.Itoa(i))
	}
	m.choice = choiceModalState{
		active: true, title: "colour for " + m.repoName(tag), options: opts, values: vals,
		apply: func(v string) tea.Cmd {
			n, _ := strconv.Atoi(v)
			return mutateThenRefresh(cl, func() error { return cl.SetRepoColor(tag, n) })
		},
	}
}

// openForgetChoice confirms forgetting a repo (it deletes the repo's agents and drops
// the registry row; the repo's files stay). Guarded behind a yes/no like agent delete.
func (m *model) openForgetChoice(tag, name string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true, title: "forget repo " + name + "? (deletes its agents; files kept)",
		options: []string{"cancel", "forget"}, values: []string{"cancel", "forget"},
		apply: func(v string) tea.Cmd {
			if v != "forget" {
				return nil
			}
			return mutateThenRefresh(cl, func() error { return cl.RepoForget(tag) })
		},
	}
}
