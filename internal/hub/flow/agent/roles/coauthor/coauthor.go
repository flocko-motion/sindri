// package: hub/flow/roles/coauthor / coauthor
// type:    logic (the coauthor's flow, declared)
// job:     the map of how a coauthor works, which is barely a map at all: its session IS the user's
// seat, so what happens next is whatever they type.
// limits:  the map. The words behind each Says live in the workflow.
package coauthor

import (
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
	"github.com/flo-at/sindri/internal/hub/flow/agent/verb"
)

const (
	Collab    = "coauthor/collab"
	Escalated = "coauthor/escalated"
)

var collab = flow.State{
	Name:  Collab,
	Title: "Working with the user",
	About: "A coauthor works directly with the user in the shared checkout. There is no task queue " +
		"behind it and nothing the hub is waiting for — it is never empty-handed, because its " +
		"session is the seat somebody is sitting in.",
	Says: says.Coauthor,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question only the user can answer"},
	},
	Verbs: flow.Offers{
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.CreateTask, flow.Stay, "propose a task"},
		{verb.Comment, flow.Stay, "comment on a task"},
		{verb.Scratch, flow.Stay, "check work out into a disposable workspace"},
		{verb.Git, flow.Stay, "read the changes"},
		{verb.Run, flow.Stay, "queue a slow build or test"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Chat, flow.Stay, "say something in the meeting room"},
		{verb.Approve, flow.Stay, "approve a pull request"},
		{verb.Reject, flow.Stay, "send a pull request back"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}

var escalated = flow.State{
	Name:  Escalated,
	Title: "Escalated",
	About: "Even sharing the user's seat, a coauthor can stop on a decision it must not make itself.",
	Says:  says.Escalated,
	Events: flow.Events{
		{cond.Resolved, Collab, "the user answered and it cleared the escalation"},
	},
	// A coauthor lands nothing through the hub — it commits with git itself — so an escalation shuts
	// nothing but leaves the question on the record for the user sitting with it.
	Verbs: flow.Offers{
		{verb.Resume, Collab, "clear the escalation once you have the answer"},
		{verb.Escalate, flow.Stay, "replace the question with a sharper one"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.CreateTask, flow.Stay, "propose a task"},
		{verb.Comment, flow.Stay, "comment on a task"},
		{verb.Scratch, flow.Stay, "check work out into a disposable workspace"},
		{verb.Git, flow.Stay, "read the changes"},
		{verb.Run, flow.Stay, "queue a slow build or test"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Chat, flow.Stay, "say something in the meeting room"},
	},
}

// Flow is the coauthor's whole map.
var Flow = []flow.State{collab, escalated}

// Start is where a coauthor with no state stored begins.
const Start = Collab
