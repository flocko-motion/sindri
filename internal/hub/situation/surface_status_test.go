package situation

import (
	"testing"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/observe"
)

// runtimeSaying is a situation whose session reports word, with nothing else in play.
func runtimeSaying(word string) Situation {
	return Situation{Observation: observe.Observation{State: observe.ParseState(word)}}
}

// TestSignedOutOutranksEveryPhase: eitri sat signed out for hours reading "idle" on the board,
// indistinguishable from a planner at rest, while hub messages piled into an input box that could
// not send them. Whatever the hub last asked of an agent, a signed-out one is not doing it — and
// unlike every other status here, it cannot even be told so.
func TestSignedOutOutranksEveryPhase(t *testing.T) {
	out := runtimeSaying("signed-out")
	for _, status := range []string{"idle", "working", "planning", "collab", "reviewing", "submitted", "stalled"} {
		if got := out.overlayRuntime(status); got != api.StatusSignedOut {
			t.Errorf("overlayRuntime(%q) over signed-out = %q, want signed-out", status, got)
		}
	}
	// A phase the session says nothing about is still the phase: a failed look reports "".
	if got := runtimeSaying("").overlayRuntime("planning"); got != "planning" {
		t.Errorf("a silent look must change nothing, got %q", got)
	}
}

// TestRuntimeDecidesIdleOrWorking: eitri was planning — the pane showed the interrupt hint, 1m41s
// and 5.1k tokens into a turn — while the board read "idle", because a planner holds no task, feature
// or PR by design and a stale guard discarded "working" against that empty column. Whether an agent
// is idle is what the SESSION says, an observed fact about the pane; what it holds is a separate
// column the board already shows, and "working, holding nothing" is a coherent, honest state.
func TestRuntimeDecidesIdleOrWorking(t *testing.T) {
	if got := runtimeSaying("working").overlayRuntime("idle"); got != "working" {
		t.Errorf("a moving pane is working even holding nothing, got %q", got)
	}
	if got := runtimeSaying("idle").overlayRuntime("working"); got != "idle" {
		t.Errorf("a still pane is idle even if the phase says working, got %q", got)
	}
	// The states that need a human are about the agent, not its workload, so they still outrank.
	for _, word := range []string{"blocked", "signed-out", "api-error"} {
		if got := runtimeSaying(word).overlayRuntime("idle"); got != word {
			t.Errorf("%s must show regardless of what is held, got %q", word, got)
		}
	}
	// A specific phase survives a session that only disagrees on the generic idle/working ambiguity:
	// the overlay replaces the generic words, never a more meaningful one.
	if got := runtimeSaying("working").overlayRuntime("planning"); got != "planning" {
		t.Errorf("a specific phase should not be flattened to the generic runtime word, got %q", got)
	}
}

// TestAnEscalatedAgentWearsTheStatus ties the durable state to the one word every front-end renders.
// Without it an escalated agent is indistinguishable from one with nothing to do, which is the whole
// failure being fixed.
func TestAnEscalatedAgentWearsTheStatus(t *testing.T) {
	asked := Situation{Escalation: "which schema?"}
	if got := asked.overlayEscalation("working"); got != api.StatusEscalated {
		t.Errorf("status = %q, want %q", got, api.StatusEscalated)
	}
	// It outranks the words describing a screen: a stalled reading is the same standing still seen
	// without the reason for it, and a runtime block is a different remedy wearing one word.
	for _, was := range []string{api.StatusStalled, api.StatusBlocked, "idle", "submitted"} {
		if got := asked.overlayEscalation(was); got != api.StatusEscalated {
			t.Errorf("overlayEscalation(%q) = %q, want %q", was, got, api.StatusEscalated)
		}
	}
	// Two still outrank it, both saying the answer cannot be DELIVERED until something else is fixed.
	for _, was := range []string{"down", api.StatusUnknown, "launching", api.StatusSignedOut} {
		if got := asked.overlayEscalation(was); got != was {
			t.Errorf("overlayEscalation(%q) = %q, want it unchanged", was, got)
		}
	}
	if got := (Situation{}).overlayEscalation("working"); got != "working" {
		t.Errorf("an agent with no escalation must be untouched, got %q", got)
	}
}
