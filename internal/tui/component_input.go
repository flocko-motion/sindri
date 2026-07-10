// package: tui / component_input
// type:    ui component (single-line input modal)
// job:     the one-line text prompt used for "tell <agent>" and new-agent name
//          entry — open it with openInput, route keys through updateInput, and
//          submitInput runs the captured action.
// limits:  captures one line; what the submitted text does is the action the
//          caller set (-> tui.go).
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// updateInput routes a keypress to the open modal: esc cancels, enter submits,
// everything else edits the field.
func (m model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode, m.inputTarget = inputNone, ""
		m.input.Blur()
		return m, nil
	case "enter":
		cmd := m.submitInput()
		m.mode, m.inputTarget = inputNone, ""
		m.input.Blur()
		return m, cmd
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// openInput starts a modal, capturing the current selection as its target.
func (m *model) openInput(mode inputMode, prompt string) {
	m.mode, m.inputTarget = mode, m.selID()
	m.input.SetValue("")
	m.input.Prompt = prompt
	m.resizeInput()
	m.input.Focus()
}

// resizeInput sizes the single-line field to the terminal (minus the prompt), so a
// longer message stays visible as you type instead of scrolling in a ~20-char box.
// Called on open and on window resize while the input is up.
func (m *model) resizeInput() {
	m.input.Width = max(20, m.w-lipgloss.Width(m.input.Prompt)-1)
}

// submitInput performs the modal's hub action with the entered value.
func (m *model) submitInput() tea.Cmd {
	v := strings.TrimSpace(m.input.Value())
	if v == "" || m.cl == nil {
		return nil
	}
	cl, target := m.cl, m.inputTarget
	if m.mode == inputTell {
		return func() tea.Msg {
			if err := cl.Tell(target, v, "user"); err != nil {
				return errModalMsg{err}
			}
			return nil
		}
	}
	return nil
}
