// package: hub/flow/run / world
// type:    logic (the world a run's conditions read, and the names its map is written in)
// job:     the vocabulary the run flow is declared in — the engine's shapes bound to a run's world,
// and the world itself, gathered once per pass.
// limits:  the vocabulary. The engine is flow/machine's, and what an action DOES is exec_act.go's.
package run

import (
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
)

// State, Events and the rest are the engine's shapes bound to a run's world, so the map below names
// them without a type parameter in sight.
type (
	State      = machine.State[World]
	Transition = machine.Transition[World]
	Condition  = machine.Condition[World]
	Action     = machine.Action
	Outcome    = machine.Outcome
	Events     = []machine.Transition[World]
)

// Orphaned is the built-in exit every acting state needs: this hub never started that action, so a
// previous one died holding it. For a run that is the whole of the old startup reconciliation.
type Orphaned = machine.Orphaned

// World is everything a run's conditions may read, gathered ONCE per pass. Conditions reaching for
// what they need separately can decide from a world that never existed.
type World struct {
	// Run is the row itself: what to execute, against which workspace, and where it stands.
	Run api.Run

	// Position is this run's place in the fleet-wide ranking, 1 for the front of the queue and 0
	// once it is no longer queued. ONE queue across every project, never one per project.
	Position int

	// SlotTaken: some run in the fleet is already executing. The queue is one slot wide, so this is
	// what a queued run at the front is still waiting on.
	SlotTaken bool

	// Stale is why this run should be dropped rather than executed — the agent that asked for it is
	// gone, or has moved on to different work since. "" when it is still worth running.
	Stale string
}
