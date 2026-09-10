package agent

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
)

// TestEveryRoleHasAFlow: an agent whose role has no map is one nobody can answer for.
func TestEveryRoleHasAFlow(t *testing.T) {
	for _, role := range Roles {
		if len(Of(role)) == 0 {
			t.Errorf("role %q has no flow", role)
		}
		if _, err := Start(role); err != nil {
			t.Errorf("role %q has no start state: %v", role, err)
		}
	}
}

// TestEveryStateNameIsUnique: two states under one name would make "what happens here" answer
// differently depending on which flow was registered first.
func TestEveryStateNameIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range All {
		if seen[s.Name] {
			t.Errorf("state %q is declared twice", s.Name)
		}
		seen[s.Name] = true
	}
}

// TestNoFlowLeavesItsOwnRole: a worker state reachable from a planner's map would be a rule written
// for somebody else answering for it. Roles are set by a human, never reached by a transition.
func TestNoFlowLeavesItsOwnRole(t *testing.T) {
	for _, role := range Roles {
		own := map[string]bool{}
		for _, s := range Of(role) {
			own[s.Name] = true
		}
		for _, s := range Of(role) {
			for _, e := range s.Events {
				if e.To != flow.Stay && !own[e.To] {
					t.Errorf("%s: %q leads to %q, outside the %s flow", role, s.Name, e.To, role)
				}
			}
			for _, v := range s.Verbs {
				if v.To != flow.Stay && !own[v.To] {
					t.Errorf("%s: %q offers %q leading to %q, outside the %s flow",
						role, s.Name, v.Verb.Name, v.To, role)
				}
			}
		}
	}
}

// TestEveryActingStateCanBeObserved is the rule the unified event list makes necessary: an action's
// outcomes are the only edge-triggered events, so a state whose exits are all outcomes is
// unreachable the moment that action dies.
func TestEveryActingStateCanBeObserved(t *testing.T) {
	for _, s := range All {
		if s.Action == nil {
			continue
		}
		observable := 0
		for _, e := range s.Events {
			if _, isOutcome := e.On.(machine.Outcome); !isOutcome {
				observable++
			}
		}
		if observable == 0 {
			t.Errorf("%s runs %q but declares no observable exit — an action that dies strands it",
				s.Name, s.Action.Name)
		}
	}
}

// TestEveryOutcomeIsHandled: an action that can finish a way its state does not name leaves the
// agent where it is, silently.
func TestEveryOutcomeIsHandled(t *testing.T) {
	for _, s := range All {
		if s.Action == nil {
			continue
		}
		for _, want := range s.Action.Outcomes {
			found := false
			for _, e := range s.Events {
				if o, ok := e.On.(machine.Outcome); ok && o.Name == want.Name {
					found = true
				}
			}
			if !found {
				t.Errorf("%s runs %q but does not say where %q leads", s.Name, s.Action.Name, want.Name)
			}
		}
	}
}

// TestEveryDeclaredThingIsUsed keeps the closed sets honest: a condition nothing watches, a verb no
// state offers, an action no state runs or a speech nothing says is a declaration nobody reads.
func TestEveryDeclaredThingIsUsed(t *testing.T) {
	usedCond, usedVerb, usedAct, usedSay := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, s := range All {
		usedSay[s.Says] = true
		if s.Action != nil {
			usedAct[s.Action.Name] = true
		}
		for _, e := range s.Events {
			usedCond[e.On.EventName()] = true
		}
		for _, v := range s.Verbs {
			usedVerb[v.Verb.Name] = true
		}
	}
	for _, c := range cond.All {
		if !usedCond[c.Name] {
			t.Errorf("condition %q is declared but no state watches it", c.Name)
		}
	}
	for _, v := range verb.All {
		if !usedVerb[v.Name] {
			t.Errorf("verb %q is declared but no state offers it", v.Name)
		}
	}
	for _, a := range act.All {
		if !usedAct[a.Name] {
			t.Errorf("action %q is declared but no state runs it", a.Name)
		}
	}
	for _, sp := range says.All {
		if !usedSay[sp] {
			t.Errorf("speech %q is declared but no state says it", sp)
		}
	}
}

// TestEveryStateReadsAsDocumentation: the fields ARE the map, so one left blank is a state nobody
// can read rather than a cosmetic omission.
func TestEveryStateReadsAsDocumentation(t *testing.T) {
	for _, s := range All {
		switch {
		case s.Title == "":
			t.Errorf("%s declares no Title", s.Name)
		case len(s.About) < 40:
			t.Errorf("%s declares no About worth reading: %q", s.Name, s.About)
		case len(s.Events) == 0:
			t.Errorf("%s declares no way out", s.Name)
		}
		for _, e := range s.Events {
			if e.Why == "" {
				t.Errorf("%s: the transition on %q carries no reason", s.Name, e.On.EventName())
			}
		}
	}
}

// TestTheMapIsPrintable: the declarations are data, so a reader answers "what happens here" from a
// printed map rather than by reconstructing it from branches.
func TestTheMapIsPrintable(t *testing.T) {
	got := machine.Table(Of("worker"))
	for _, want := range []string{"worker/idle", "Idle", "-> worker/assigning"} {
		if !strings.Contains(got, want) {
			t.Errorf("the printed map is missing %q:\n%s", want, got)
		}
	}
}
