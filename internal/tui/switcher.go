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

	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/client"
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
	var opts, vals []string
	for _, p := range m.state.Projects {
		label := m.repoName(p.Tag)
		if p.Path == m.root {
			label += " ✓"
		}
		opts = append(opts, label)
		vals = append(vals, p.Path)
	}
	m.choice = choiceModalState{
		active: true, title: "Switch repo", options: opts, values: vals,
		apply: func(path string) tea.Cmd { return func() tea.Msg { return switchRepoMsg(path) } },
	}
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
	m.cursor = [3]int{}
	m.flash = "switched to " + name
	return waitForState(m.ch, m.gen)
}
