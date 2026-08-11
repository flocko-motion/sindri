// package: tui / priority choice
// type:    ui (Tasks tab pick-one modals)
// job:     the priority picker for the selected task and, where open tasks sit below it, the scope
// step that says how far the rating carries — and states what carrying it would actually do,
// so the wider choice cannot read as releasing work it only reordered.
// limits:  labels and key-to-call plumbing only; what a scope reaches is the hub's
// (-> api.PriorityEffect, workflow.SetPriority) and the wording is theme's.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// openPriorityChoice opens the priority picker for a task. With open tasks below it the rating can
// reach them, so the pick is followed by the scope; with nothing below there is only the one task.
func (m *model) openPriorityChoice(id string) {
	cl := m.cl
	cascade := api.PriorityEffect(m.state.Tasks, id)
	vals := make([]string, len(theme.PriorityWords))
	for i, w := range theme.PriorityWords {
		vals[i] = theme.PriorityCode(w)
	}
	m.choice = choiceModalState{
		active: true, title: "priority for " + id,
		options: theme.PriorityWords, values: vals,
		apply: func(code string) tea.Cmd {
			// The scope step is a screen, not a hub call, so it opens whether or not there is a client
			// to write through — only the write itself needs one.
			if cascade.Children > 0 {
				return func() tea.Msg { return openPriorityScopeMsg{id: id, code: code} }
			}
			if cl == nil {
				return nil
			}
			return setPriorityCmd(cl, id, code, api.ScopeTask)
		},
	}
}

// openPriorityScopeChoice asks how far the rating carries, its title stating what carrying it would
// ACTUALLY do (-> theme.PriorityScopeNote): under an open parent the children are worked as one
// package, so a rating orders them rather than releasing them.
func (m *model) openPriorityScopeChoice(id, code string) {
	cl := m.cl
	cascade := api.PriorityEffect(m.state.Tasks, id)
	vals := make([]string, len(api.PriorityScopes))
	for i, s := range api.PriorityScopes {
		vals[i] = string(s)
	}
	m.choice = choiceModalState{
		active: true, title: theme.PriorityLabel(code) + " for " + id + " — how far?",
		note:    theme.PriorityScopeNote(cascade),
		options: theme.PriorityScopeLabels(cascade), values: vals,
		apply: func(scope string) tea.Cmd {
			if cl == nil {
				return nil
			}
			return setPriorityCmd(cl, id, code, api.PriorityScope(scope))
		},
	}
}

// setPriorityCmd writes the rating and refreshes the board once.
func setPriorityCmd(cl *client.HTTP, id, code string, scope api.PriorityScope) tea.Cmd {
	return mutateThenRefresh(cl, func() error { return cl.SetPriority(id, code, scope) })
}
