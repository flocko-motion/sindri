// package: tui / agents tab — the unsent-line warning
// type:    logic (test)
// job:     hold that a line left unsent in an agent's pane reaches the board, since it stops every
// push the hub makes and the only way to find it used to be capturing the pane by hand.
// limits:  the rendering. Detecting the line is the harness's (-> harness.Observe).
package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestTheObservationSaysWhenALineIsWaitingUnsent: thrain's pane held "restart the hub and try
// again", typed and never sent, so every push to it failed for two hours — the mail announcing a
// user's answer to an escalation among them. Nothing on the board said so.
func TestTheObservationSaysWhenALineIsWaitingUnsent(t *testing.T) {
	quiet := observationLine(api.AgentView{ObservedAt: "14:07", Runtime: "idle"})
	if strings.Contains(quiet, "unsent") {
		t.Errorf("an ordinary pane must say nothing about it: %q", quiet)
	}
	blocked := observationLine(api.AgentView{ObservedAt: "14:07", Runtime: "idle", InputPending: true})
	if !strings.Contains(blocked, "unsent") {
		t.Errorf("a pane holding a line must say so: %q", blocked)
	}
	if !strings.Contains(blocked, "no push lands") {
		t.Errorf("it must say what the line COSTS, or it reads as a detail: %q", blocked)
	}
}
