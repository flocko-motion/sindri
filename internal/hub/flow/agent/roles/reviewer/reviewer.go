// package: hub/flow/agent/roles/reviewer / reviewer
// type:    logic (the reviewer's flow, declared)
// job:     the map of how a reviewer works. It has ONE workspace, so it holds exactly one pull
// request at a time and a second assignment is not a queue.
// limits:  the map. The words behind each Says live in the workflow.
package reviewer

import (
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
)

const (
	Idle      = "reviewer/idle"
	Taking    = "reviewer/taking"
	Reviewing = "reviewer/reviewing"
	Dropping  = "reviewer/dropping"
)

var idle = flow.State{
	Name:  Idle,
	Title: "Reviewer holding no pull request",
	About: "Free, so the oldest unclaimed review is its next one — which is also how a review filed " +
		"while no reviewer was running gets picked up later.",
	Says: says.NoReviews,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question"},
		{cond.Retired, Retired, "a human wound it down"},
		{cond.ReviewHeld, Reviewing, "it already holds one, however it got there"},
		{cond.MailWaiting, Mail, "it has mail it has not read, so it is not done"},
		{cond.ClearArmed, Clearing, "a human armed a context clear — it fires before any claim"},
		// Through the CLEAR, not around it: a review is read whole, so the session is discarded before
		// the branch arrives rather than by whoever hands it over. Every route passes the same way,
		// which is what stops two of them clearing the session twice.
		{cond.ReviewWaiting, Clearing, "a pull request is waiting, and a review is read on a fresh session"},
		// The pod comes LAST, after everything it could be answering for. A review waiting on a pool
		// whose every pod was reclaimed is what the wake is FOR: the row stays unclaimed until one is
		// up, so the evidence a reviewer was needed outlives the start.
		{cond.StartAsked, Launching, "a human asked for this pod"},
		{cond.NeededWhileAsleep, Launching, "a review is waiting, this pod was reclaimed, and nobody awake would take it"},
		{cond.StopAsked, Stopping, "a human asked for this pod back"},
		{cond.Reclaimable, Stopping, "it has held nothing long enough that its pod is worth taking back"},
	},
	Verbs: flow.Offers{
		{verb.Mail, "read your mailbox"},
		{verb.Task, "read the backlog"},
		{verb.Comment, "comment on what it has ruled on"},
		{verb.Log, "record a note"},
		{verb.Fyi, "one note to the user"},
		{verb.Escalate, "stop on a question"},
	},
}

var taking = flow.State{
	Name:   Taking,
	Title:  "Being handed a pull request",
	Action: act.TakeReview,
	About: "The hub is claiming a review for this reviewer and putting the branch in its workspace. " +
		"The claim comes first, so a session discarded mid-way cannot lose it.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Reviewing, "the pull request is checked out and served"},
		{act.Nothing, Idle, "somebody else took it first"},
		{act.Failed, Idle, "the hand-over failed; it will be offered again"},
		{flow.Orphaned{}, Idle, "the hub restarted mid-hand-over"},
	},
	Verbs: flow.Offers{{verb.Log, "record a note"}},
}

var reviewing = flow.State{
	Name:  Reviewing,
	Title: "Reading a pull request",
	About: "The reviewer holds one pull request whose branch sits in its one workspace. A PR that " +
		"settles while it is reading releases the hold: a verdict on it now decides nothing.",
	Says: says.Reviewing,
	Events: flow.Events{
		{cond.Escalated, Escalated, "it stopped on a question"},
		{cond.ReviewOvertaken, Dropping, "the pull request settled before a verdict"},
		{cond.ReviewDone, Idle, "it ruled on the one it held, and a verdict is where a review ends"},
		{cond.AsleepHolding, Launching, "it holds this review and its pod is gone — a claim must not outlive the pod it was made for"},
	},
	Verbs: flow.Offers{
		{verb.Approve, "approve what you have read"},
		{verb.Reject, "send it back with feedback"},
		{verb.Git, "read the diff"},
		{verb.Task, "read the task it answers"},
		{verb.Comment, "comment on the task"},
		{verb.Run, "queue a slow build or test"},
		{verb.Log, "record a note"},
		{verb.Fyi, "one note to the user"},
		{verb.Mail, "read your mailbox"},
		{verb.Escalate, "stop on a question"},
	},
}

var dropping = flow.State{
	Name:   Dropping,
	Title:  "Releasing an overtaken review",
	Action: act.DropReview,
	About: "The pull request this reviewer held was merged, scrapped or withdrawn before it gave a " +
		"verdict. The hold is released and the reviewer is told, rather than left to produce a " +
		"verdict that would overwrite the outcome.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Idle, "the hold is released"},
		{flow.Orphaned{}, Idle, "the hub restarted mid-release"},
	},
	Verbs: flow.Offers{{verb.Log, "record a note"}},
}

// Flow is the reviewer's whole map.
var Flow = []flow.State{
	idle, mail, taking, reviewing, dropping, launching, stopping, clearing, escalated, retired,
}

// Start is where a reviewer with no state stored begins.
const Start = Idle
