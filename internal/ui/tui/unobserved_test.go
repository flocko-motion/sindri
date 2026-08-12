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

// TestAttachNeverAsksTheBoardFirst: the status is the watchdog's last sweep, and on a loaded host it
// calls a live agent down — eitri read "down" while its session answered fine. Refusing on that
// costs the access; attempting costs a moment, and the attempt is also the better probe. So no
// status, observed or not, may turn a keypress into a refusal.
func TestAttachNeverAsksTheBoardFirst(t *testing.T) {
	for _, s := range []string{"down", api.StatusUnknown, "launching", "stopping", "working", "idle"} {
		m := agentAt(s)
		cmd := m.onKey(keyAttach)
		if cmd == nil {
			t.Errorf("status %q: attach was refused rather than attempted", s)
		}
		if m.errText != "" {
			t.Errorf("status %q: nothing is known to be wrong yet, but it said %q", s, m.errText)
		}
	}
}
