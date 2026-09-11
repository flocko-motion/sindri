// package: hub/flow/agent/roles/planner / planner
// type:    logic (the planner's flow, declared)
// job:     the map of how a planner works. It never touches the backlog: its work arrives as a
// CONVERSATION, so the only questions are whether a plan is in hand and whether one has shipped.
// limits:  the map. The words behind each Says live in the workflow.
package planner

import (
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
)

const (
	Idle      = "planner/idle"
	Planning  = "planner/planning"
	Submitted = "planner/submitted"
	Disowning = "planner/disowning"
)

var idle = flow.State{
	Name:  Idle,
	Title: "Planner at rest",
	About: "A planner never grabs a backlog task. Work reaches it as a conversation, so at rest it " +
		"is pointed at whatever the user has already said rather than at a queue. It is TOLD that on " +
		"arrival: a planner reaches this state with a session somebody just emptied, and asking it to " +
		"ask would be a round trip for an answer the hub is already holding.",
	Says:  says.Planner,
	Tells: true,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question"},
		{cond.HoldsBacklogWork, Disowning, "a backlog task is on its row, and a planner holds none"},
		{cond.MailWaiting, Mail, "it has mail it has not read, so it is not done"},
		{cond.Retired, Retired, "a human wound it down"},
		{cond.ClearArmed, Clearing, "a human armed a context clear"},
		{cond.StartAsked, Launching, "a human asked for this pod"},
		{cond.StopAsked, Stopping, "a human asked for this pod back"},
		{cond.Reclaimable, Stopping, "it has held nothing long enough that its pod is worth taking back"},
		// A planner's work IS the conversation, so a session with something in it is one with work in
		// hand. It declares nothing about where it stands: the session is the fact, and reading a fact
		// is the machine's job.
		{cond.InConversation, Planning, "something has been said into its session"},
	},
	Verbs: flow.Offers{
		{verb.CreateTask, "propose a task"},
		{verb.Comment, "comment on a task"},
		{verb.Task, "read the backlog"},
		{verb.Rebase, "align onto the reference branch"},
		{verb.Mail, "read your mailbox"},
		{verb.Log, "record a note"},
		{verb.Fyi, "one note to the user"},
		{verb.Escalate, "stop on a question"},
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
		{cond.HoldsBacklogWork, Disowning, "a backlog task is on its row, and a planner holds none"},
		{cond.ConversationOver, Idle, "its session holds nothing, so the plan it was inside is gone with it"},
		// A planner holds no backlog task, so every moment is a leaf boundary and an armed clear
		// fires wherever it stands. It lands at rest afterwards on purpose: the conversation it was
		// inside is what the clear discarded, so telling it to carry on with one would name a
		// session that no longer exists.
		{cond.ClearArmed, Clearing, "a human armed a context clear"},
	},
	Verbs: flow.Offers{
		{verb.Openspec, "ship the spec edits as a pull request"},
		{verb.CreateTask, "propose a task"},
		{verb.Task, "read the backlog"},
		{verb.Comment, "comment on a task"},
		{verb.Mail, "read your mailbox"},
		{verb.Log, "record a note"},
		{verb.Fyi, "one note to the user"},
		{verb.Rebase, "align onto the reference branch"},
		{verb.Escalate, "stop on a question"},
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
		{verb.Mail, "read your mailbox"},
		{verb.Task, "read the backlog"},
		{verb.Comment, "comment on a task"},
		{verb.Log, "record a note"},
		{verb.Fyi, "one note to the user"},
		{verb.Escalate, "stop on a question"},
	},
}

// disowning: a backlog task ended up on a planner's row, which is a claim nobody should have made.
var disowning = flow.State{
	Name:   Disowning,
	Title:  "Letting go of a backlog task",
	Action: act.Disown,
	About: "A planner's work arrives as a conversation, so a backlog task on its row is an invalid " +
		"claim however it got there. The task goes back to the backlog and the planner goes back to " +
		"resting — noticed by the planner's own map wherever it stands, rather than by a sweep that " +
		"only ever ran at the hub's start.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Idle, "the task is back in the backlog"},
		{act.Failed, Idle, "it could not be put back — the reason is on its record"},
		{flow.Orphaned{}, Idle, "the hub restarted mid-release"},
	},
	Verbs: flow.Offers{{verb.Log, "record a note"}},
}

// Flow is the planner's whole map.
var Flow = []flow.State{
	idle, planning, submitted, disowning, mail, launching, stopping, clearing, escalated, retired,
}

// Start is where a planner with no state stored begins.
const Start = Idle
