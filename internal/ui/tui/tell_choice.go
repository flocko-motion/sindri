// package: tui / tell choice
// type:    ui (Agents tab confirm modal)
// job:     what to do when the user tells an agent whose pane reads signed out — send regardless,
// restart it first, or cancel — since the reading is a look at a screen and the person typing
// usually knows better than a banner left in the transcript.
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

// openTellChoice offers the answers to a signed-out pane. SEND leads, because the reading is most
// often stale: the banner sits in the transcript long after the turn that produced it, so an agent
// working normally still reads signed-out — balin answered a message while the board said otherwise.
// The restart is kept but demoted and told the truth about: it re-reads the credentials the hub
// already staged, so unless those CHANGED it hands the process the token it is holding. When they do
// change the hub restarts the agent itself (-> credwatch.revive), which is why this rarely helps.
func (m *model) openTellChoice(name, msg string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true,
		title:  name + "'s pane reads signed out — send anyway?",
		note: "That reading is a look at a SCREEN, and the /login banner stays in the transcript long after " +
			"the turn that printed it — so an agent working normally reads signed-out too.\n" +
			"If it really is at a /login prompt, what you send waits in its input box until it is not.\n" +
			"A restart only helps if the host's token CHANGED since that process started; when it does, the " +
			"hub restarts the agent itself. Renewing the token on the host is the fix that works.",
		options: []string{"cancel", "send anyway", "restart " + name + " first, then send"},
		values:  []string{"cancel", api.SignedOutSend, api.SignedOutRestart},
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
