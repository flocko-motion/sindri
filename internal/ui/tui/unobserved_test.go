package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
)

// agentAt builds an Agents-tab model holding one agent in the given status, with a client so the
// launch path runs rather than returning early.
func agentAt(status string) model {
	m := newModel(&client.HTTP{}, nil, "")
	m.tab, m.scopeRepo = 1, false
	m.state = api.BoardState{Agents: []api.AgentView{{Name: "dvalin", Project: "repo", Status: status}}}
	return m
}

// TestStartStopNeverStopsAnUnobservedAgent: S switched on the status word and ended in a default
// meaning "running", so an agent nothing had looked at took the STOP branch — S on a fresh agent
// asked the hub to stop something that was never started. Not-yet-observed is where "down" used to
// be, so it belongs on the launch branch, which is also what the user pressing S is asking for.
func TestStartStopNeverStopsAnUnobservedAgent(t *testing.T) {
	m := agentAt(api.StatusUnknown)
	m.agentStartStop()
	if strings.Contains(m.flash, "stopping") {
		t.Errorf("S on an unobserved agent tried to stop it: %q", m.flash)
	}
	if !strings.Contains(m.flash, "launching") {
		t.Errorf("S on an unobserved agent should launch it, got flash %q", m.flash)
	}
	// And a genuinely running agent is still stopped by the same key.
	m = agentAt("working")
	m.agentStartStop()
	if !strings.Contains(m.flash, "stopping") {
		t.Errorf("S on a running agent should stop it, got %q", m.flash)
	}
}

// TestAttachRefusedForAnUnobservedAgent: the guards asked `Status == "down"`, so "unknown" walked
// past the friendly message and attached to a container that may not exist — the user got a raw
// podman error instead of a sentence. Every tab that offers attach must refuse it.
func TestAttachRefusedForAnUnobservedAgent(t *testing.T) {
	m := agentAt(api.StatusUnknown)
	if cmd := m.onKey(keyAttach); cmd != nil {
		t.Error("attach on an unobserved agent should be refused, not executed")
	}
	if m.errText == "" {
		t.Fatal("a refused attach must say why")
	}
	// It must not claim the agent is down — that is the claim the hub declined to make.
	if strings.Contains(m.errText, "is down") {
		t.Errorf("an unobserved agent is not known to be down: %q", m.errText)
	}
	if !strings.Contains(m.errText, "observed") {
		t.Errorf("the refusal should say nothing has looked yet, got %q", m.errText)
	}
}

// TestAttachStillRefusesADownAgentTheSameWay guards the wording the refusal already had, so
// grouping the two statuses did not cost the clear message for the case that was already handled.
func TestAttachStillRefusesADownAgentTheSameWay(t *testing.T) {
	m := agentAt("down")
	if cmd := m.onKey(keyAttach); cmd != nil {
		t.Error("attach on a down agent should be refused")
	}
	if !strings.Contains(m.errText, "is down") || !strings.Contains(m.errText, keyStartS) {
		t.Errorf("a down agent should still be told to start it with %q, got %q", keyStartS, m.errText)
	}
}

// TestAttachRefusalWordsEveryNonRunningStatus: the three tabs share one refusal builder so they
// cannot drift, and every status that is not up must produce a sentence rather than fall through.
func TestAttachRefusalWordsEveryNonRunningStatus(t *testing.T) {
	for _, s := range []string{"down", api.StatusUnknown, "launching", "stopping"} {
		if got := attachRefusal("dvalin", s, "'S'"); got == "" || !strings.Contains(got, "dvalin") {
			t.Errorf("attachRefusal(%q) = %q, want a sentence naming the agent", s, got)
		}
	}
}
