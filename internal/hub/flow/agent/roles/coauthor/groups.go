// package: hub/flow/agent/roles/coauthor / groups
// type:    logic (the coauthor's regions, declared)
// job:     the regions a coauthor's states sit in, and the exits the seat holds for what sits in it.
// limits:  the regions; the engine resolves inheritance (-> machine.Group, machine.Exits).
package coauthor

import (
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
)

const (
	GroupSeat = "coauthor/seat"
	GroupPod  = "coauthor/pod"
)

// Groups is the coauthor's regions. The seat's exits are everything a human does to it, in the
// order they win: a question, a wind-down, a clear, then the pod.
var Groups = []flow.Group{
	{Name: GroupSeat, Title: "At the user's seat",
		About: "The session is the seat somebody is sitting in; only a human moves a coauthor out of it.",
		First: flow.Events{
			{On: cond.Escalated, To: Escalated, Why: "it stopped on a question only the user can answer"},
			{On: cond.Retired, To: Retired, Why: "a human wound it down"},
			{On: cond.ClearArmed, To: Clearing, Why: "a human armed a context clear"},
			{On: cond.StartAsked, To: Launching, Why: "a human asked for this pod"},
			{On: cond.StopAsked, To: Stopping, Why: "a human asked for this pod back"},
		}},
	{Name: GroupPod, Title: "The pod and its session",
		About: "Starting, stopping and clearing the pod, and the stops a human puts it in."},
}
