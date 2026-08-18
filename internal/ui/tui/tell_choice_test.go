package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
)

// signedOutBoard is a board with one agent whose pane reads /login and one that is working.
func signedOutBoard() api.BoardState {
	return api.BoardState{
		Projects: []api.Project{{Tag: "rdb", Path: "/r/ranke-db"}},
		Agents: []api.AgentView{
			{Project: "rdb", Repo: "ranke-db", Name: "eitri", Status: api.StatusSignedOut},
			{Project: "rdb", Repo: "ranke-db", Name: "nori", Status: "working", Task: "sd-1"},
		},
	}
}

// TestTellingASignedOutAgentAsksInsteadOfRefusing is the bug: the message used to come back as an
// error naming a restart the modal could not perform. Now the restart is one of the answers, and
// so is sending regardless — the pane reading may be older than what the user knows.
func TestTellingASignedOutAgentAsksInsteadOfRefusing(t *testing.T) {
	m := newModel(nil, nil, "/r/ranke-db")
	m.cl = &client.HTTP{} // never dialled: these cases end before any command runs
	m.state = signedOutBoard()
	m.openInput(inputTell, "tell eitri: ")
	m.inputTarget = "eitri"
	m.input.SetValue("carry on")

	if cmd := m.submitInput(); cmd != nil {
		t.Error("a signed-out agent takes the message through the choice, not straight to the hub")
	}
	if !m.choice.active {
		t.Fatal("the choice must open — the user is the one who can answer for the pane")
	}
	var restart, send bool
	for _, v := range m.choice.values {
		restart = restart || v == api.SignedOutRestart
		send = send || v == api.SignedOutSend
	}
	if !restart || !send {
		t.Errorf("both answers must be offered, got %q", m.choice.values)
	}
	if m.choice.values[0] != "cancel" {
		t.Errorf("cancel must sit under the cursor, got %q", m.choice.values)
	}
	if m.choice.values[1] != api.SignedOutRestart {
		t.Errorf("the restart is the remedy that works, so it leads the answers: %q", m.choice.values)
	}
	if !strings.Contains(m.choice.title+m.choice.note, "restart") {
		t.Error("the modal must say what a restart does, since that is why it is offered")
	}
}

// TestTellingAWorkingAgentJustSends: the choice is for the one state that raises the question, and
// every other message goes as it always did.
func TestTellingAWorkingAgentJustSends(t *testing.T) {
	m := newModel(nil, nil, "/r/ranke-db")
	m.cl = &client.HTTP{}
	m.state = signedOutBoard()
	m.openInput(inputTell, "tell nori: ")
	m.inputTarget = "nori"
	m.input.SetValue("carry on")

	m.submitInput() // the returned command is never run — only the routing is under test
	if m.choice.active {
		t.Error("an agent that is not signed out must not be asked about")
	}
}
