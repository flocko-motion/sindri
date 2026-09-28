package fleet

import (
	"testing"

	agentflow "github.com/flo-at/sindri/internal/hub/flow/agent"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/worker"
	"github.com/flo-at/sindri/internal/hub/flowtest"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// TestEveryRolesGraphLeadsOnlyToItsOwnStates: the view draws one role's map, so an edge whose target
// is not in it would dangle — it draws as a ghost node, which is a map bug worth failing on here.
func TestEveryRolesGraphLeadsOnlyToItsOwnStates(t *testing.T) {
	for _, role := range agentflow.Roles {
		g, err := AgentGraph(role)
		if err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		names := map[string]bool{}
		for _, s := range g.States {
			names[s.Name] = true
		}
		if !names[g.Start] {
			t.Errorf("%s starts in %q, which its graph does not have", role, g.Start)
		}
		for _, s := range g.States {
			for i, ev := range s.Events {
				if ev.To != "" && !names[ev.To] {
					t.Errorf("%s: %s exit %d (%s) leads to %q, outside its own flow", role, s.Name, i+1, ev.On, ev.To)
				}
				if ev.Kind != "outcome" && ev.Kind != "condition" && ev.Kind != "orphaned" {
					t.Errorf("%s: %s exit %d has kind %q", role, s.Name, i+1, ev.Kind)
				}
			}
		}
	}
}

// TestARecordedMoveResolvesToItsEdge pins the history's click-to-edge against the rows the machine
// really writes: the view parses the detail text, so a change to how a move is recorded must break
// here and not silently in a browser.
func TestARecordedMoveResolvesToItsEdge(t *testing.T) {
	d := &flowtest.Hub{Root: t.TempDir()}
	e, ps, _ := podFleet(t, d, store.Agent{Name: "durin", Role: "worker", Workspace: ".worktrees/durin"})
	flowtest.Place(t, ps, store.AgentState{Agent: "durin", Phase: worker.Idle})
	if err := e.roleAct().AskStop("repo", "durin"); err != nil {
		t.Fatal(err)
	}
	e.Look("repo", "durin")

	v, err := e.AgentSubject("repo", "durin")
	if err != nil {
		t.Fatal(err)
	}
	g, err := AgentGraph("worker")
	if err != nil {
		t.Fatal(err)
	}
	moves := 0
	for _, h := range v.History {
		if h.Reason != "moved" {
			continue
		}
		if h.Move == nil {
			t.Errorf("a recorded move did not parse: %q", h.Detail)
			continue
		}
		moves++
		found := false
		for _, s := range g.States {
			if s.Name != h.Move.From {
				continue
			}
			for _, ev := range s.Events {
				if ev.On == h.Move.On && ev.To == h.Move.To {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("move %+v names no edge of the worker's graph", *h.Move)
		}
	}
	if moves == 0 {
		t.Fatalf("a stop asked of an idle worker recorded no move; history = %+v", v.History)
	}
}

// TestAMoveWithoutTheShapeParsesToNothing: a row the parse does not recognise yields no edge, so the
// view never highlights one it guessed.
func TestAMoveWithoutTheShapeParsesToNothing(t *testing.T) {
	for _, detail := range []string{"", "worker/idle", "worker/idle: no arrow here", "worker/idle: ev -> "} {
		if m := parseMove(detail); m != nil {
			t.Errorf("parseMove(%q) = %+v, want nil", detail, *m)
		}
	}
}
