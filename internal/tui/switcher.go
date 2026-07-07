// package: tui / switcher
// type:    ui (repo switcher)
// job:     the repo picker — choose which repo the board scopes to. Agents and PRs
//          stay global; the Tasks tab (and container/attach) follow the selected
//          repo. Selecting re-subscribes /events with the chosen repo so its tasks
//          stream in, bumping the generation so the old stream's messages are dropped.
// limits:  just the switch; the generic pick-one chrome is component_choice.
package tui

import (
	"context"
	"sort"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/hub/store"
)

// switchRepoMsg asks the model to re-scope the board to a repo (by its path).
type switchRepoMsg string

// openSwitcher opens the repo picker over the board's known projects, marking the
// current one.
func (m *model) openSwitcher() {
	if len(m.state.Projects) == 0 {
		m.flash = "no repos yet"
		return
	}
	projects := m.switcherOrder()
	var opts, vals []string
	for _, p := range projects {
		label := m.repoName(p.Tag)
		if m.repoHasLiveAgent(p.Tag) {
			label += " ●" // a repo where work is happening right now
		}
		if p.Path == m.root {
			label += " ✓"
		}
		// Each entry in its repo's bright shade, so the picker mirrors the header's
		// per-repo colours. padTrunc is ANSI-aware and the (plain) name stays
		// contiguous, so the typeahead filter still matches.
		opts = append(opts, m.repoStyle(p.Tag).Render(label))
		vals = append(vals, p.Path)
	}
	m.choice = choiceModalState{
		active: true, title: "Switch repo", options: opts, values: vals, filterable: true,
		apply: func(path string) tea.Cmd { return func() tea.Msg { return switchRepoMsg(path) } },
	}
}

// switcherOrder ranks the known repos for the picker: repos with a live agent first
// (that's where work is happening), then by recency (most-recently-used), then
// alphabetically by name — so the relevant repos are always near the top of a
// possibly-long list.
func (m *model) switcherOrder() []store.Project {
	ps := append([]store.Project(nil), m.state.Projects...)
	sort.SliceStable(ps, func(i, j int) bool {
		li, lj := m.repoHasLiveAgent(ps[i].Tag), m.repoHasLiveAgent(ps[j].Tag)
		if li != lj {
			return li // live agents first
		}
		if ps[i].LastUsed != ps[j].LastUsed {
			return ps[i].LastUsed > ps[j].LastUsed // most recent first
		}
		return m.repoName(ps[i].Tag) < m.repoName(ps[j].Tag)
	})
	return ps
}

// repoHasLiveAgent reports whether any agent in the repo is currently running — read
// from the already-global board snapshot, so ordering needs no extra hub call.
func (m *model) repoHasLiveAgent(tag string) bool {
	for _, a := range m.state.Agents {
		if a.Project == tag && a.Status != "down" {
			return true
		}
	}
	return false
}

// switchRepo re-subscribes /events to path so the Tasks tab (and container/attach)
// scope to it while Agents/PRs stay global. It cancels the current subscription and
// dials afresh, bumping the generation so the abandoned stream's trailing messages
// (including its close) are ignored.
func (m *model) switchRepo(path string) tea.Cmd {
	if path == "" || path == m.root {
		return nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.cl = client.Dial(path)
	ch, err := m.cl.Watch(ctx)
	if err != nil {
		m.err = err
		return tea.Quit
	}
	name := path
	for _, p := range m.state.Projects {
		if p.Path == path {
			name = m.repoName(p.Tag)
			break
		}
	}
	m.ch = ch
	m.gen++
	m.root = path
	m.cursor = [4]int{}
	m.flash = "switched to " + name
	return waitForState(m.ch, m.gen)
}
