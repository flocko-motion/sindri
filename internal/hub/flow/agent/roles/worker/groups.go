// package: hub/flow/agent/roles/worker / groups
// type:    logic (the worker's regions, declared)
// job:     the regions a worker's states sit in, and the exits each one holds for every state inside
// it — declared once, so no state in a region can forget one.
// limits:  the regions. A state's own exits are its own file's; the engine resolves inheritance
// (-> machine.Group, machine.Exits).
package worker

import (
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
)

// The regions. A region whose states the hub acts in holds no exits: an exit on an acting state
// would cut its action short, so those states only sit there to be drawn together.
const (
	GroupClaim       = "worker/claim"
	GroupTask        = "worker/task"
	GroupHandsOn     = "worker/hands-on"
	GroupInReview    = "worker/in-review"
	GroupFeature     = "worker/feature"
	GroupFeatureHeld = "worker/feature-held"
	GroupPod         = "worker/pod"
)

// escalatedFirst is what every region an agent is at work in holds first: a human stepping in comes
// before anything the work itself would do next.
var escalatedFirst = flow.Transition{On: cond.Escalated, To: Escalated, Why: "it stopped on a question only the user can answer"}

// Groups is the worker's regions.
var Groups = []flow.Group{
	{Name: GroupClaim, Title: "Being handed work",
		About: "The hub selects a unit, prepares the session for it, and only then says what it is. The " +
			"worker is acted upon throughout and hears once, at the hand-over that ends the chain."},
	{Name: GroupTask, Title: "Holding a task",
		About: "A leaf task is in hand: the worker is at it, answering for it, or waiting on a verdict, and " +
			"the hub steps in only to refresh, submit, reset or promote the branch it is on."},
	{Name: GroupHandsOn, In: GroupTask, Title: "At work on it",
		About: "The agent is the actor here. An escalation comes before anything the work would do next.",
		First: flow.Events{escalatedFirst}},
	{Name: GroupInReview, In: GroupTask, Title: "Waiting on a verdict",
		About: "What it filed is with the gate or a reviewer. An escalation comes before the verdict.",
		First: flow.Events{escalatedFirst}},
	{Name: GroupFeature, Title: "Holding a feature",
		About: "A whole feature is in hand, worked one child at a time; the hub lets go of it when it lands " +
			"without the worker or its tree is split."},
	{Name: GroupFeatureHeld, In: GroupFeature, Title: "Between its children",
		About: "No child is in hand. An escalation comes first, then a feature that is no longer on its row.",
		First: flow.Events{
			escalatedFirst,
			{On: cond.HoldsNothing, To: Idle, Why: "the feature it was working through is no longer on its row"},
		}},
	{Name: GroupPod, Title: "The pod and its session",
		About: "Starting, stopping and clearing the pod, and the stops a human or its mail put it in."},
}
