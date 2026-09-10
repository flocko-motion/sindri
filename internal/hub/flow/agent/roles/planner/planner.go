// package: hub/flow/roles/planner / planner
// type:    logic (the planner's flow, declared)
// job:     the map of how a planner works. It never touches the backlog: its work arrives as a
// CONVERSATION, so the only questions are whether a plan is in hand and whether one has shipped.
// limits:  the map. The words behind each Says live in the workflow.
package planner

import (
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
	"github.com/flo-at/sindri/internal/hub/flow/agent/verb"
)

const (
	Idle      = "planner/idle"
	Planning  = "planner/planning"
	Submitted = "planner/submitted"
	Clearing  = "planner/clearing"
	Escalated = "planner/escalated"
)

var idle = flow.State{
	Name:  Idle,
	Title: "Planner at rest",
	About: "A planner never grabs a backlog task. Work reaches it as a conversation, so at rest it " +
		"is pointed at whatever the user has already said rather than at a queue.",
	Says: says.Planner,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question"},
		{cond.ClearArmed, Clearing, "a human armed a context clear"},
	},
	Verbs: flow.Offers{
		{verb.State, Planning, "say you are working a plan"},
		{verb.CreateTask, flow.Stay, "propose a task"},
		{verb.Comment, flow.Stay, "comment on a task"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Rebase, flow.Stay, "align onto the reference branch"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Fyi, flow.Stay, "one note to the user"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}

var planning = flow.State{
	Name:  Planning,
	Title: "Planning with the user",
	About: "The planner has work in hand — a brief it was given, or a conversation it is inside. " +
		"The hub waits on nothing from it and has nothing to add until it ships or declares itself idle.",
	Says: says.Planning,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question"},
	},
	Verbs: flow.Offers{
		{verb.Plan, Submitted, "ship the spec edits as a pull request"},
		{verb.CreateTask, flow.Stay, "propose a task"},
		{verb.State, Idle, "say you are done"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Comment, flow.Stay, "comment on a task"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Fyi, flow.Stay, "one note to the user"},
		{verb.Rebase, flow.Stay, "align onto the reference branch"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}

var submitted = flow.State{
	Name:  Submitted,
	Title: "Planner waiting on a verdict",
	About: "The planner shipped its spec edits as a pull request, and that PR IS the review — there " +
		"is nothing else to ask anyone to read.",
	Says: says.AwaitVerdict,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question"},
		{cond.PRSettled, Idle, "the proposal landed or was withdrawn"},
		{cond.OwnPRRejected, Planning, "it came back with feedback to answer"},
	},
	Verbs: flow.Offers{
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Comment, flow.Stay, "comment on a task"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Fyi, flow.Stay, "one note to the user"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}

var clearing = flow.State{
	Name:   Clearing,
	Title:  "Having its session cleared",
	Action: act.Clear,
	About:  "A human armed a clear on this planner's session, and the hub is waiting for it to land.",
	Says:   says.Preparing,
	Events: flow.Events{
		{act.Done, Idle, "the session is empty"},
		{act.Failed, Idle, "the clear never landed — carry on regardless"},
		{flow.Orphaned{}, Idle, "the hub restarted mid-clear"},
	},
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}},
}

var escalated = flow.State{
	Name:  Escalated,
	Title: "Escalated",
	About: "The planner stopped on a question only the user can answer, and it is repeated on every " +
		"ask: one relaunched mid-escalation remembers nothing of how it got here.",
	Says: says.Escalated,
	Events: flow.Events{
		{cond.Resolved, Planning, "the user answered and it cleared the escalation"},
	},
	// Only the LANDING verbs are held — shipping a proposal. Reading, proposing tasks and talking
	// to the user are how a planner gets its answer in the first place.
	Verbs: flow.Offers{
		{verb.Resume, Planning, "clear the escalation once you have the answer"},
		{verb.Escalate, flow.Stay, "replace the question with a sharper one"},
		{verb.Git, flow.Stay, "read the changes"},
		{verb.Revoke, flow.Stay, "withdraw a proposal you have out"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.CreateTask, flow.Stay, "propose a task"},
		{verb.Comment, flow.Stay, "comment on a task"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Fyi, flow.Stay, "one note to the user"},
		{verb.Rebase, flow.Stay, "align onto the reference branch"},
		{verb.State, flow.Stay, "say where you are"},
		{verb.Chat, flow.Stay, "say something in the meeting room"},
	},
}

// Flow is the planner's whole map.
var Flow = []flow.State{idle, planning, submitted, clearing, escalated}

// Start is where a planner with no state stored begins.
const Start = Idle
