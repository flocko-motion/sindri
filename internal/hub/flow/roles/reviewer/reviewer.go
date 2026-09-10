// package: hub/flow/roles/reviewer / reviewer
// type:    logic (the reviewer's flow, declared)
// job:     the map of how a reviewer works. It has ONE workspace, so it holds exactly one pull
// request at a time and a second assignment is not a queue.
// limits:  the map. The words behind each Says live in the workflow.
package reviewer

import (
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/act"
	"github.com/flo-at/sindri/internal/hub/flow/cond"
	"github.com/flo-at/sindri/internal/hub/flow/says"
	"github.com/flo-at/sindri/internal/hub/flow/verb"
)

const (
	Mail      = "reviewer/mail"
	Idle      = "reviewer/idle"
	Taking    = "reviewer/taking"
	Reviewing = "reviewer/reviewing"
	Dropping  = "reviewer/dropping"
	Clearing  = "reviewer/clearing"
	Escalated = "reviewer/escalated"
	Retired   = "reviewer/retired"
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
	},
	Verbs: flow.Offers{
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Comment, flow.Stay, "comment on what it has ruled on"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Fyi, flow.Stay, "one note to the user"},
		{verb.Escalate, Escalated, "stop on a question"},
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
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}},
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
	},
	Verbs: flow.Offers{
		{verb.Approve, Idle, "approve what you have read"},
		{verb.Reject, Idle, "send it back with feedback"},
		{verb.Git, flow.Stay, "read the diff"},
		{verb.Task, flow.Stay, "read the task it answers"},
		{verb.Comment, flow.Stay, "comment on the task"},
		{verb.Run, flow.Stay, "queue a slow build or test"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Fyi, flow.Stay, "one note to the user"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Escalate, Escalated, "stop on a question"},
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
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}},
}

var clearing = flow.State{
	Name:   Clearing,
	Title:  "Having its session cleared",
	Action: act.Clear,
	About:  "The session is discarded before the next pull request, so each review is read whole.",
	Says:   says.Preparing,
	Events: flow.Events{
		{act.Done, Taking, "the session is empty; a review can be handed over"},
		{act.Failed, Taking, "the clear never landed — a crowded reviewer beats a PR nobody was told about"},
		{flow.Orphaned{}, Idle, "the hub restarted mid-clear"},
	},
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}},
}

var escalated = flow.State{
	Name:  Escalated,
	Title: "Escalated",
	About: "The reviewer stopped on a question only the user can answer; it is repeated on every ask.",
	Says:  says.Escalated,
	Events: flow.Events{
		{cond.Resolved, Reviewing, "the user answered and it cleared the escalation"},
	},
	// Only the LANDING verbs are held — the verdict itself. A reviewer stopped on a question still
	// reads the diff and the task, and still says what it has found.
	Verbs: flow.Offers{
		{verb.Resume, Reviewing, "clear the escalation once you have the answer"},
		{verb.Escalate, flow.Stay, "replace the question with a sharper one"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Comment, flow.Stay, "comment on the task"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Fyi, flow.Stay, "one note to the user"},
		{verb.Run, flow.Stay, "queue a slow build or test"},
		{verb.Git, flow.Stay, "read the diff"},
		{verb.Chat, flow.Stay, "say something in the meeting room"},
	},
}

var retired = flow.State{
	Name:  Retired,
	Title: "Retired",
	About: "A human wound this reviewer down: no further review is handed to it. Retirement means " +
		"no new claim for a reviewer exactly as it does for a worker.",
	Says: says.Retired,
	Events: flow.Events{
		{cond.BackInService, Idle, "a human brought it back"},
	},
	Verbs: flow.Offers{{verb.Mail, flow.Stay, "read your mailbox"}},
}

// mail: unread mail means it is not done, and an agent that is not done is not cleared for a PR.
var mail = flow.State{
	Name:  Mail,
	Title: "Mail to read",
	About: "The reviewer has unread mail. It is not done until it has read it, and a reviewer that " +
		"is not done is not prepared for the next pull request — which is what stops a clear landing " +
		"on top of a message nobody has seen.",
	Says: says.Mail,
	Events: flow.Events{
		{cond.MailRead, Idle, "the mailbox is empty again"},
	},
	Verbs: flow.Offers{
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Escalate, Escalated, "stop on a question"},
	},
}

// Flow is the reviewer's whole map.
var Flow = []flow.State{idle, mail, taking, reviewing, dropping, clearing, escalated, retired}

// Start is where a reviewer with no state stored begins.
const Start = Idle
