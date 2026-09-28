// package: hub/flow/agent/roles/coauthor / coauthor
// type:    logic (the coauthor's flow, declared)
// job:     the map of how a coauthor works, which is barely a map at all: its session IS the user's
// seat, so what happens next is whatever they type.
// limits:  the map. The words behind each Says live in the workflow.
package coauthor

import (
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
)

const Collab = "coauthor/collab"

var collab = flow.State{
	WhenIdle: flow.LetItRest, // the user types here, and silence between turns is the ordinary case
	Name:     Collab,
	Title:    "Working with the user",
	About: "A coauthor works directly with the user in the shared checkout. There is no task queue " +
		"behind it and nothing the hub is waiting for — it is never empty-handed, because its " +
		"session is the seat somebody is sitting in. It TELLS NOTHING on arrival: six lifecycle " +
		"states lead back here, so anything said would be said again for reading a message or " +
		"stopping a pod — and what there is to say is standing in the agent's own brief already, " +
		"which a cleared session keeps.",
	Says: says.Coauthor,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question only the user can answer"},
		{cond.Retired, Retired, "a human wound it down"},
		{cond.ClearArmed, Clearing, "a human armed a context clear"},
		{cond.StartAsked, Launching, "a human asked for this pod"},
		{cond.StopAsked, Stopping, "a human asked for this pod back"},
	},
	Verbs: flow.Offers{
		{verb.Task, "read the backlog"},
		{verb.CreateTask, "propose a task"},
		{verb.Comment, "comment on a task"},
		{verb.Scratch, "check work out into a disposable workspace"},
		{verb.Git, "read the changes"},
		{verb.Run, "queue a slow build or test"},
		{verb.Log, "record a note"},
		{verb.Mail, "read your mailbox"},
		{verb.Meeting, "say something in the meeting room"},
		{verb.Approve, "approve a pull request"},
		{verb.Reject, "send a pull request back"},
		{verb.Escalate, "stop on a question"},
	},
}

// Flow is the coauthor's whole map.
var Flow = []flow.State{collab, launching, stopping, clearing, escalated, retired}

// Start is where a coauthor with no state stored begins.
const Start = Collab
