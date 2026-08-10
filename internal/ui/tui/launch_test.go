package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
)

// TestFailedLaunchIsNotSilent is the bug this file exists for. The launch used to run in a bare
// goroutine with its error dropped (`_ = cl.Launch(...)`) and its output sent to io.Discard, so an
// agent that failed to start just sat at "down" saying nothing — no modal, no flash, no log line.
// A launch failure must produce something a user can read.
func TestFailedLaunchIsNotSilent(t *testing.T) {
	m := newModel(nil, nil, "")
	m.flash = "launching dvalin… (a first run builds the image, which takes a while)"
	tm, _ := m.Update(launchedMsg{
		name: "dvalin",
		log:  "STEP 3/9: RUN go build ./...\nno required module provides package foo",
		err:  errors.New("exit status 1"),
	})
	got := tm.(model)
	if !got.modal {
		t.Fatal("a failed launch showed no modal — the agent would sit at 'down' unexplained")
	}
	body := strings.Join(got.modalOverride, "\n")
	// The build log is the diagnosis; "exit status 1" on its own says nothing about what broke.
	if !strings.Contains(body, "no required module provides package foo") {
		t.Errorf("the launch output is not shown, so the reason is lost:\n%s", body)
	}
	if !strings.Contains(got.modalOverrideTitle, "dvalin") || !strings.Contains(got.modalOverrideTitle, "exit status 1") {
		t.Errorf("the title should name the agent and the failure, got %q", got.modalOverrideTitle)
	}
	// The "launching…" flash must not survive a failure: left up, it reads as still in progress.
	if strings.Contains(got.flash, "launching") {
		t.Errorf("the launching flash outlived the failure: %q", got.flash)
	}
}

// TestFailedLaunchWithNoOutputStillReports: a launch can fail before it writes anything (no engine,
// no socket). An empty modal would be its own kind of silence, so it says the output was empty.
func TestFailedLaunchWithNoOutputStillReports(t *testing.T) {
	m := newModel(nil, nil, "")
	tm, _ := m.Update(launchedMsg{name: "dvalin", err: errors.New("podman: not found")})
	got := tm.(model)
	if !got.modal {
		t.Fatal("a failed launch with no output showed no modal")
	}
	if body := strings.TrimSpace(strings.Join(got.modalOverride, "\n")); body == "" {
		t.Error("the modal is empty — an empty box explains nothing")
	}
	if !strings.Contains(got.modalOverrideTitle, "podman: not found") {
		t.Errorf("the reason is not in the title either: %q", got.modalOverrideTitle)
	}
}

// TestSuccessfulLaunchSaysSoAndRefreshes: the happy path reports and re-reads the board, so the row
// moves off "down" without waiting for the next poll.
func TestSuccessfulLaunchSaysSoAndRefreshes(t *testing.T) {
	m := newModel(&client.HTTP{}, nil, "")
	tm, cmd := m.Update(launchedMsg{name: "dvalin", log: "launched — coming up"})
	got := tm.(model)
	if got.modal {
		t.Error("a successful launch should not interrupt with a modal")
	}
	if !strings.Contains(got.flash, "dvalin") {
		t.Errorf("a successful launch said nothing: %q", got.flash)
	}
	if cmd == nil {
		t.Error("a successful launch should refresh the board")
	}
}

// TestCreatingAnAgentLaunchesIt: registration and launch are two hub calls, and creating an agent
// must lead to the second one — that is what makes the TUI and `agent new` agree.
func TestCreatingAnAgentLaunchesIt(t *testing.T) {
	m := newModel(&client.HTTP{}, nil, "")
	tm, cmd := m.Update(agentCreatedMsg("dvalin"))
	if cmd == nil {
		t.Fatal("a newly created agent is never launched — it would stay down")
	}
	if flash := tm.(model).flash; !strings.Contains(flash, "dvalin") {
		t.Errorf("the launch is not announced: %q", flash)
	}
	// A first run builds the image, which is slow; silence reads as nothing happening.
	if flash := tm.(model).flash; !strings.Contains(flash, "builds the image") {
		t.Errorf("the flash does not warn that a first launch is slow: %q", flash)
	}
}

// TestStartingADownAgentKeepsTheLaunchOutput: the S toggle used to discard the launch output too, so
// a failed image build surfaced as a bare "exit status 1". It goes through the same path now.
func TestStartingADownAgentKeepsTheLaunchOutput(t *testing.T) {
	m := newModel(&client.HTTP{}, nil, "")
	m.tab, m.scopeRepo = 1, false
	m.state = api.BoardState{Agents: []api.AgentView{{Name: "dvalin", Status: "down"}}}
	if cmd := m.agentStartStop(); cmd == nil {
		t.Fatal("starting a down agent produced no command")
	}
	if !strings.Contains(m.flash, "builds the image") {
		t.Errorf("starting a down agent does not go through the launch path that keeps output: %q", m.flash)
	}
}
