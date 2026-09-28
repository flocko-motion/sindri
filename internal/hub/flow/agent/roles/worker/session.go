// package: hub/flow/agent/roles/worker / session
// type:    logic (the worker's own session state, declared)
// job:     the one thing done to a worker's session that no other role needs — switching the model
// under it for the tier of the work being handed over.
// limits:  the map. The states every role shares are built through one factory (-> lifecycle.go).
package worker

import (
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
)

// The two preparation states, in the order they run. Work is already HELD in both: the hub selects,
// then prepares, then instructs, so neither can be reached from a state where nothing was claimed.
const (
	Preparing = "worker/preparing"
	Retiering = "worker/retiering"
)

// preparing: the session is emptied for work already claimed. The first step after a claim, and
// unconditional — a clear with nothing to drop answers at once, so nothing has to ask first.
var preparing = flow.State{
	WhenIdle: flow.LetItRest, // the hub is emptying its session
	Name:     Preparing,
	Title:    "Having its session prepared",
	Action:   act.Prepare,
	About: "The work is selected and the session is being emptied for it. Work arrives WHOLE, so the " +
		"unit before it is context to drop rather than condense — and nothing is said into a session " +
		"until every step of the preparation has run.",
	Says: says.Preparing,
	Events: flow.Events{
		{On: act.Done, To: Retiering, Why: "the session is empty; the model comes next"},
		// Nothing runs on past a clear that would not land. The step behind this one types `/model`,
		// which on a session still holding its history opens a dialog that swallows the brief.
		{On: act.Failed, To: Escalated, Why: "the clear would not land, so a human has to look"},
		{On: cond.SessionGone, To: Working, Why: "the pod went away; it still holds the work and is told on its next ask"},
		{On: flow.Orphaned{}, To: Working, Why: "the hub restarted mid-preparation, and the claim stands"},
	},
	Verbs: flow.Offers{{Verb: verb.Log, Why: "record a note"}},
}

// retiering: the session is put on the model its claimed work is rated for. The second step, and
// unconditional for the same reason — a switch to the model already running answers at once.
var retiering = flow.State{
	WhenIdle: flow.LetItRest, // the hub is switching its model
	Name:     Retiering,
	Title:    "Having its model changed",
	Action:   act.Retier,
	About: "The work in hand is rated for a tier, and the session is put on that tier's model — " +
		"which narrates and restarts it on the way through. Nothing said into this session survives.",
	Says: says.Preparing,
	Events: flow.Events{
		{On: act.Done, To: HandingOver, Why: "the session is on the right model; the work can be handed over"},
		// No route onward on a failure. Typing two lines into a pane is not a thing that fails, so one
		// that did is a fault in the harness — and handing the work over anyway would run it on
		// whatever model the session happened to hold, which is the outcome this exists to prevent.
		{On: act.Failed, To: Escalated, Why: "the switch would not land, so a human has to look"},
		{On: cond.SessionGone, To: Working, Why: "the pod went away; it still holds the work and is told on its next ask"},
		{On: flow.Orphaned{}, To: Working, Why: "the hub restarted mid-switch, and the claim stands"},
	},
	Verbs: flow.Offers{{Verb: verb.Log, Why: "record a note"}},
}
