// package: hub/flow/agent/roles/planner / groups
// type:    logic (the planner's regions, declared)
// job:     the regions a planner's states sit in, and the exits each holds for what sits in it.
// limits:  the regions; the engine resolves inheritance (-> machine.Group, machine.Exits).
package planner

import (
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
)

const (
	GroupSeat         = "planner/seat"
	GroupConversation = "planner/conversation"
	GroupPod          = "planner/pod"
)

// Groups is the planner's regions. Nested so the order holds: an escalation first everywhere at the
// seat, and a backlog task on its row second wherever a conversation could be under way. Disowning is
// the hub's own action and sits in none, since an exit on it would cut the release short.
var Groups = []flow.Group{
	{Name: GroupSeat, Title: "At the planner's seat",
		About: "Resting, planning, or waiting on the verdict for what it shipped. An escalation comes " +
			"before anything any of those would do next.",
		First: flow.Events{
			{On: cond.Escalated, To: Escalated, Why: "it stopped on a question"},
		}},
	{Name: GroupConversation, In: GroupSeat, Title: "Open to a conversation",
		About: "A planner's work arrives as a conversation, so a backlog task on its row is an invalid " +
			"claim, noticed here before anything else.",
		First: flow.Events{
			{On: cond.HoldsBacklogWork, To: Disowning, Why: "a backlog task is on its row, and a planner holds none"},
		}},
	{Name: GroupPod, Title: "The pod and its session",
		About: "Starting, stopping and clearing the pod, and the stops a human puts it in."},
}
