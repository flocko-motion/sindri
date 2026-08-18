// package: tui / component_input
// type:    ui component (single-line input modal)
// job:     the one-line text prompt used for "tell <agent>" and new-agent name
// entry — open it with openInput, route keys through updateInput, and
// submitInput runs the captured action.
// limits:  captures one line; what the submitted text does is the action the
// caller set (-> tui.go).
package tui

import (
	"fmt"
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
	switch m.mode {
	case inputTell:
		// A signed-out agent takes the message through a choice rather than a refusal: the remedy
		// the refusal names is one of the options (-> openTellChoice).
		if m.agentReadsSignedOut(target) {
			m.openTellChoice(target, v)
			return nil
		}
		return tellCmd(cl, target, v)
	case inputMail:
		// No signed-out question here, unlike tell: mail never touches the session, so a pane that
		// cannot receive anything is exactly the case mail is FOR.
		return mutateThenRefresh(cl, func() error { return cl.MailAgent(target, v) })
	case inputMailReply:
		// The recipient comes from the message, not from this prompt — which is why the target is an id.
		var id int64
		if _, err := fmt.Sscanf(target, "%d", &id); err != nil {
			return nil
		}
		return mutateThenRefresh(cl, func() error { return cl.ReplyToMail(id, v) })
	case inputRunCommand:
		// Against this repo's own checkout, the target only a human has — an agent's workspace is
		// the agent's to queue. Not scheduled inline: the queue answers at once with a position,
		// and the refresh is what puts the new row on the tab.
		return func() tea.Msg {
			if _, err := cl.ScheduleRun(v, "", "", ""); err != nil {
				return errModalMsg{err}
			}
			st, _ := cl.State()
			return polledMsg(st)
		}
	case inputComment:
		// Refreshed after: a GitHub issue's thread is re-read on the way back, so the comment
		// appears with the author and timestamp the source gave it.
		return func() tea.Msg {
			if err := cl.AddTaskComment(target, v); err != nil {
				return errModalMsg{err}
			}
			st, _ := cl.State()
			return polledMsg(st)
		}
	}
	return nil
}
