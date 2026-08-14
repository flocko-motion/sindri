// package: tui / tell choice
// type:    ui (Agents tab confirm modal)
// job:     what to do when the user tells an agent whose pane reads signed out — restart it and
// send, send regardless, or cancel — since the reading is a look at a screen and the person
// typing may know the token was renewed a moment ago.
// limits:  labels and choice-to-call plumbing only; whether the answer comes into play, and the
// restart itself, are the hub's (-> agent.Service.Tell).
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
)

// agentReadsSignedOut reports whether the board's last word on this agent is a signed-out pane.
func (m model) agentReadsSignedOut(name string) bool {
	for _, a := range m.state.Agents {
		if a.Name == name {
			return a.Status == api.StatusSignedOut
		}
	}
	return false
}

// openTellChoice offers the answers to a signed-out pane. The restart comes first among them
// because it is the remedy that works — a restarted process re-reads the credentials the hub keeps
// staged — and cancel keeps the cursor's default harmless, as every other confirm here does.
func (m *model) openTellChoice(name, msg string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true,
		title:  name + " reads signed out — restart it and send, or send anyway?",
		note: "Nothing typed at a /login prompt is sent: the message would sit in its input box, unread.\n" +
			"A restart makes the process re-read the credentials the hub keeps staged; the session resumes.\n" +
			"That reading is a look at its pane, so if you have just renewed the host's token, send anyway.",
		options: []string{"cancel", "restart " + name + ", then send", "send anyway"},
		values:  []string{"cancel", api.SignedOutRestart, api.SignedOutSend},
		apply: func(v string) tea.Cmd {
			if v == "cancel" {
				return nil
			}
			// Refreshed after: a restart changes the agent's status, and a send that lands is a
			// line in its activity log.
			return mutateThenRefresh(cl, func() error { return cl.Tell(name, msg, "user", v) })
		},
	}
}

// tellCmd delivers a message with no signed-out answer attached — the ordinary case, where the
// hub's refusal still stands if the pane turns out to say /login.
func tellCmd(cl *client.HTTP, name, msg string) tea.Cmd {
	return func() tea.Msg {
		if err := cl.Tell(name, msg, "user", api.SignedOutRefuse); err != nil {
			return errModalMsg{err}
		}
		return nil
	}
}
