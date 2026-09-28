// package: hub/flow/agent/idle / idle_test
// type:    logic (test)
// job:     hold that every state of every role says what a still screen means there, so no agent can
// stop doing anything in a state nobody thought about.
// limits:  the declaration. What the hub DOES about it is the sweep's (-> hub/tick_stalled).
package idle

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/flow/machine"
)

// TestEveryStateSaysWhatIdleMeansThere: an agent doing nothing is a problem in most states and
// correct in a few, and only the state knows which. Left undeclared, the answer would be whatever
// the zero value happened to be — which is how a worker sat in `worker/mail` doing nothing at all,
// for two hours, while the sweep that would have prodded it did not cover that state.
func TestEveryStateSaysWhatIdleMeansThere(t *testing.T) {
	for _, s := range byPhase {
		if s.WhenIdle == machine.IdleUndeclared {
			t.Errorf("%s does not say what a still screen means there — declare WhenIdle: "+
				"machine.Nudge where the agent is the one expected to move, machine.LetItRest where "+
				"it is waiting on somebody else", s.Name)
		}
	}
}

// TestTheStatesThatNudgeAreTheOnesTheAgentOwes pins the classification itself, which the surface's
// own test cannot see: it is handed a rule rather than these maps. Each of these is a state an agent
// was found doing nothing in, or one where doing nothing is the whole point.
func TestTheStatesThatNudgeAreTheOnesTheAgentOwes(t *testing.T) {
	for phase, want := range map[string]bool{
		// The agent owes the next move.
		"worker/working":          true, // it holds the work and is inside it
		"worker/reworking":        true, // the feedback is its to answer
		"worker/resolving":        true, // the conflict markers are in its workspace
		"worker/not-done":         true, // thrain: mail delivered, and reading it is all it owes
		"worker/feature-done":     true,
		"worker/between-subtasks": true,
		"reviewer/reviewing":      true,
		"planner/planning":        true,
		// Somebody else owes it.
		"worker/idle":          false, // nothing selected for it yet
		"worker/submitted":     false, // a reviewer owes the verdict
		"worker/gating":        false, // the fleet's one gate slot
		"worker/feature-gated": false, // the user's approval
		"worker/assigning":     false, // the hub is choosing
		"worker/preparing":     false, // the hub is emptying its session
		"worker/escalated":     false, // the user owes the answer
		"worker/retired":       false,
		"coauthor/collab":      false, // the user types here
		"reviewer/idle":        false,
	} {
		if _, got := Rule(phase); got != want {
			t.Errorf("Rule(%q) nudges = %v, want %v", phase, got, want)
		}
	}
	// A phase nothing declares rests: the rule claims only what a map actually said.
	if _, got := Rule("worker/invented"); got {
		t.Error("an unknown phase must not be claimed as a stall")
	}
}
