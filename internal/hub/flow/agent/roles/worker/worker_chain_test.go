package worker

import (
	"testing"

	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
)

// chainOrder is the claim chain in the order the hub runs it: select, prepare, instruct. The two
// claims share rank 0 because neither leads to the other — a leaf and a subtask are two ways in.
var chainOrder = map[string]int{
	Assigning: 0, Picking: 0, Preparing: 1, Retiering: 2, HandingOver: 3,
}

// TestTheClaimChainOnlyRunsForward is the four-day bug as a property of the map. Every edge that
// stays inside the chain leads to a LATER step, so no step can put the agent back at one it has
// already passed. The circle that ran at twelve seconds a lap was exactly this: preparation led to
// the claim, the claim bounced a feature holder back to rest, and rest asked for preparation again.
func TestTheClaimChainOnlyRunsForward(t *testing.T) {
	by := map[string]flow.State{}
	for _, s := range Flow {
		by[s.Name] = s
	}
	for name, rank := range chainOrder {
		s, declared := by[name]
		if !declared {
			t.Fatalf("%s is not in the worker's flow — the guard is reading names nothing declares", name)
		}
		for _, e := range s.Events {
			to, inChain := chainOrder[e.To]
			if !inChain {
				continue // leaving the chain is how it ends; only a step BACK is the fault
			}
			if to <= rank {
				t.Errorf("%s leads to %s, which is not further along the chain (%d -> %d). The hub "+
					"selects, prepares and instructs in that order; an edge back is a lap", name, e.To, rank, to)
			}
		}
	}
}

// TestPreparationIsEnteredByAClaimAlone holds the other half. A state the worker RESTS in — one the
// hub runs no action in — may send it to a claim and no further: an edge from rest into the middle
// of the chain is a second way in, and a second way in is a way round.
func TestPreparationIsEnteredByAClaimAlone(t *testing.T) {
	resting := 0
	for _, s := range Flow {
		if s.Action != nil {
			continue
		}
		resting++
		for _, e := range s.Events {
			if e.To == Preparing || e.To == Retiering || e.To == HandingOver {
				t.Errorf("%s rests, yet leads to %s. Preparation follows a claim; reached from rest it "+
					"is a way back into it", s.Name, e.To)
			}
		}
	}
	if resting < 5 {
		t.Fatalf("only %d resting states scanned — the guard is not reading the flow", resting)
	}
}

// TestEveryChainStepEndsAtTheWork: each step declares where it lands when its pod or its hub goes
// away mid-run, and the answer is never a state that holds nothing. The work is already CLAIMED by
// then, so an agent dropped back to idle would be one holding work the map says it does not.
func TestEveryChainStepEndsAtTheWork(t *testing.T) {
	for _, s := range Flow {
		if chainOrder[s.Name] < 1 { // the claims themselves hold nothing yet, so idle is right for them
			continue
		}
		for _, e := range s.Events {
			if _, isOutcome := e.On.(machine.Outcome); isOutcome {
				continue
			}
			if e.To == Idle {
				t.Errorf("%s falls back to %s on %q, but the work is claimed by then — it belongs at the "+
					"work, which is where its next ask is answered from", s.Name, e.To, e.On.EventName())
			}
		}
	}
}

// parks reports a state an agent can stand in indefinitely: nothing but a CONDITION moves it out.
// An acting state always leaves by its own outcome, so it can hold nobody however it was entered —
// which is why the rule below is about this shape rather than about a list somebody keeps.
func parks(s flow.State) bool {
	for _, e := range s.Events {
		if _, isOutcome := e.On.(flow.Outcome); isOutcome && e.To != flow.Stay {
			return false
		}
	}
	return true
}

// holdsNothingIsFine names every PARKING state an agent may legitimately stand in with neither a
// task nor a feature on its row, and why. Everything else that parks MEANS an agent with work in
// hand, so standing there holding none is a contradiction with no way out of it.
var holdsNothingIsFine = map[string]string{
	Idle: "holding nothing is what the state IS",
	// A pull request outlives the state row that filed it: an author whose row was cleared still owes
	// its reviewer an answer, and idle routes it to these deliberately. They are anchored by the PR.
	Submitted: "answerable for a pull request, which is what it holds here",
	Reworking: "the round it is answering belongs to that PR, not to a row",
	Resolving: "the conflict is on the branch of a PR it filed",
	// The shared lifecycle states: every role reaches them, and a worker holding nothing is ordinary.
	Escalated: "stopped on a question, whatever it holds",
	Retired:   "wound down",
}

// TestNoWorkStateHoldsAnEmptyHandedAgent: a state that means "this agent is at work" AND that only a
// condition can move it out of must say where an agent with no work goes, or it holds one for ever.
// thrain raised an escalation from `assigning`, before anything was claimed; clearing it returned it
// to `working`, whose every other exit reads the work it was assumed to hold — and it sat there,
// shown as busy, holding nothing, until somebody looked at the database.
func TestNoWorkStateHoldsAnEmptyHandedAgent(t *testing.T) {
	for _, s := range Flow {
		if !parks(s) {
			continue // its action lands it somewhere; no agent stays here
		}
		if why, fine := holdsNothingIsFine[s.Name]; fine {
			if why == "" {
				t.Errorf("%s is exempt with no reason given", s.Name)
			}
			continue
		}
		leads := ""
		for _, e := range s.Events {
			if c, ok := e.On.(flow.Condition); ok && c.Name == cond.HoldsNothing.Name {
				leads = e.To
			}
		}
		if leads == "" {
			t.Errorf("%s means an agent with work in hand and only a condition can move it out, but "+
				"it never asks whether there is any work — one that arrives empty-handed stays for "+
				"ever. Declare the %s exit, or exempt it with the reason holding nothing is legitimate",
				s.Name, cond.HoldsNothing.Name)
		}
	}
}

// TestEveryExemptionNamesAParkingState keeps the exemption list honest: a state renamed, deleted, or
// given an action that lands it leaves an entry excusing nothing, and the next state to take that
// name inherits it.
func TestEveryExemptionNamesAParkingState(t *testing.T) {
	parking := map[string]bool{}
	for _, s := range Flow {
		if parks(s) {
			parking[s.Name] = true
		}
	}
	for name := range holdsNothingIsFine {
		if !parking[name] {
			t.Errorf("%s is exempt but is not a state that can park an agent", name)
		}
	}
}
