// package: hub/flow/agent/roles/reviewer / groups
// type:    logic (the reviewer's regions, declared)
// job:     the regions a reviewer's states sit in, and the exits the seat holds for what sits in it.
// limits:  the regions; the engine resolves inheritance (-> machine.Group, machine.Exits).
package reviewer

import (
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
)

const (
	GroupSeat = "reviewer/seat"
	GroupPod  = "reviewer/pod"
)

// Groups is the reviewer's regions. The hub's own actions — taking and dropping a review — sit in
// none: an exit on an acting state would cut its action short.
var Groups = []flow.Group{
	{Name: GroupSeat, Title: "Free or reading",
		About: "The reviewer is free for the next review, or reading the one it holds. An escalation " +
			"comes before anything either would do next.",
		First: flow.Events{
			{On: cond.Escalated, To: Escalated, Why: "it stopped on a question"},
		}},
	{Name: GroupPod, Title: "The pod and its session",
		About: "Starting, stopping and clearing the pod, and the stops a human or its mail put it in."},
}
