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

const Retiering = "worker/retiering"

// retiering: the model under the agent is changed for the work coming.
var retiering = flow.State{
	Name:   Retiering,
	Title:  "Having its model changed",
	Action: act.Retier,
	About: "The work being handed over is rated for a different tier, so the model under the agent " +
		"is switched — which narrates and restarts the session on its way through. Nothing said into " +
		"this session survives it.",
	Says: says.Preparing,
	Events: flow.Events{
		{On: act.Done, To: Assigning, Why: "the model is switched; work can be handed over"},
		{On: act.Failed, To: Assigning, Why: "the switch never landed — hand the work over on the old model"},
		{On: flow.Orphaned{}, To: Idle, Why: "the hub restarted mid-switch"},
		{On: cond.SessionGone, To: Idle, Why: "the pod went away mid-switch"},
	},
	Verbs: flow.Offers{{Verb: verb.Log, Why: "record a note"}},
}
