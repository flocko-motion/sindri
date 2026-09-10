package run

import (
	"context"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/flow/machine"
)

// TestTheRunMapAssembles runs the engine's own check over this map: unique names, declared targets,
// an implementation per action, every outcome handled, and — the one this map exists to satisfy — a
// condition exit on every acting state, so a run whose hub died is reachable again.
func TestTheRunMapAssembles(t *testing.T) {
	m, err := machine.New(t.Context(), machine.Config[World]{
		States: Flow, Start: Start,
		Gather: func(string) (World, error) { return World{}, nil },
		Stored: func(string) (string, time.Time, error) { return Start, time.Time{}, nil },
		Move:   func(_, _, _, _ string) error { return nil },
		Do: map[string]machine.Doer[World]{
			Execute.Name: func(context.Context, World) (Outcome, error) { return Ran, nil },
			Drop.Name:    func(context.Context, World) (Outcome, error) { return Dropped, nil },
		},
	})
	if err != nil {
		t.Fatalf("the run map does not assemble: %v", err)
	}
	defer m.Close()
}

// TestEveryConditionIsWatched: a question nothing asks is a rule nobody reads, and the tolerance
// declared on it would set a cadence for no state at all.
func TestEveryConditionIsWatched(t *testing.T) {
	watched := map[string]bool{}
	for _, s := range Flow {
		for _, e := range s.Events {
			watched[e.On.EventName()] = true
		}
	}
	for _, c := range Conditions {
		if !watched[c.Name] {
			t.Errorf("condition %q is declared but no state watches it", c.Name)
		}
	}
	for _, a := range Actions {
		found := false
		for _, s := range Flow {
			if s.Action != nil && s.Action.Name == a.Name {
				found = true
			}
		}
		if !found {
			t.Errorf("action %q is declared but no state runs it", a.Name)
		}
	}
}

// TestEveryConditionDeclaresATolerance: cadence lives on the QUESTION, so one left at zero would
// silently take the machine's default rather than saying how stale its answer may be.
func TestEveryConditionDeclaresATolerance(t *testing.T) {
	for _, c := range Conditions {
		if c.Within <= 0 {
			t.Errorf("condition %q declares no tolerance", c.Name)
		}
	}
}
