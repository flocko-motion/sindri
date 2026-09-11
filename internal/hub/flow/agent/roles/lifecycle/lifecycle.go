// package: hub/flow/agent/roles/lifecycle / lifecycle
// type:    logic (the states every role shares, built once)
// job:     build the session and pod lifecycle states — launching, stopping, clearing, mail,
// escalated, retired — from each role's own destinations. WHERE one leads differs by role; what can
// move a subject out of it does not, so the event list is written here and nowhere else.
// limits:  the shared states. A role's own work states are its own file's (-> roles/<role>).
package lifecycle

import (
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
)

// The suffix each lifecycle state carries. Named so a role spells its own states by joining rather
// than by repeating a word four times, and so a reader greps one place for "every clearing state".
const (
	LaunchingIn = "/launching"
	StoppingIn  = "/stopping"
	ClearingIn  = "/clearing"
	MailIn      = "/mail"
	EscalatedIn = "/escalated"
	RetiredIn   = "/retired"
)

// note is the one verb every lifecycle state allows: an agent standing in one of them is being acted
// upon, and the only thing left to it is saying what it is seeing.
var note = flow.Offers{{Verb: verb.Log, Why: "record a note"}}

// Launching brings a stopped pod back up, because something its role answers for is waiting. The
// destinations are one: whatever the role rests in, which re-decides the moment the pod is up.
//
// Every parameter is a positional argument rather than a field of a struct, so a role that omits
// one does not compile. That is the failure this whole shape exists to prevent — the bug behind it
// was a single missing edge in one of two copies of a hand-written state.
func Launching(name, resting string) flow.State {
	return flow.State{
		Name:   name,
		Title:  "Being started",
		Action: act.Launch,
		About: "Work this agent's role answers for is waiting and its pod was reclaimed, so the hub " +
			"is bringing it back. Nothing is claimed until it is up: a claim made against a dead pod " +
			"erases the very evidence that an agent was needed, which is how a filed review went " +
			"unread by every reviewer at once.",
		Says: says.Preparing,
		Events: flow.Events{
			{On: act.Done, To: resting, Why: "the pod is up; where it rests decides what happens next"},
			{On: act.Failed, To: resting, Why: "the pod would not start — the reason is on its record"},
			{On: cond.SessionUp, To: resting, Why: "the pod is running, however it got there"},
			{On: cond.LaunchOverdue, To: resting, Why: "the launch has stood too long to still be one"},
			{On: flow.Orphaned{}, To: resting, Why: "the hub restarted mid-launch"},
		},
		Verbs: note,
	}
}

// Stopping reclaims a pod the agent has held nothing in past the idle threshold. The session is
// preserved, so the cost of being wrong is the next start's latency and nothing else.
func Stopping(name, resting string) flow.State {
	return flow.State{
		Name:   name,
		Title:  "Being reclaimed",
		Action: act.Stop,
		About: "This agent has held nothing long enough that its pod is worth taking back. A stop " +
			"preserves the session, so reclaiming costs only the next start's latency — which is why " +
			"idleness alone triggers it and memory pressure never does.",
		Says: says.Preparing,
		Events: flow.Events{
			{On: act.Done, To: resting, Why: "the pod is reclaimed"},
			{On: act.Failed, To: resting, Why: "the stop did not land — it stands where it was"},
			{On: cond.SessionGone, To: resting, Why: "the pod is gone, however it went"},
			{On: flow.Orphaned{}, To: resting, Why: "the hub restarted mid-stop"},
		},
		Verbs: note,
	}
}

// Clearing discards the session before whatever comes next. ready is where a clear that landed leads
// — the hand-over it was taken for; resting is where one that could not happen leaves the agent.
func Clearing(name, ready, resting string) flow.State {
	return flow.State{
		Name:   name,
		Title:  "Having its session cleared",
		Action: act.Clear,
		About: "A clear was typed into the session and the hub is waiting for the reading to fall — " +
			"the clear having HAPPENED, where a sleep only assumes it. Whatever comes next arrives " +
			"whole behind it, so the previous unit is context to drop rather than condense.",
		Says: says.Preparing,
		Events: flow.Events{
			{On: act.Done, To: ready, Why: "the session is empty; the hand-over can run"},
			{On: act.Failed, To: ready, Why: "the clear never landed — a crowded session beats none at all"},
			{On: cond.SessionGone, To: resting, Why: "the pod went away mid-clear, so nothing can be handed over"},
			{On: flow.Orphaned{}, To: resting, Why: "the hub restarted mid-clear"},
		},
		Verbs: note,
	}
}

// Mail is unread mail, which means the agent is not DONE — and one that is not done is not prepared
// for anything new, which is what stops a clear landing on top of a message nobody has seen.
func Mail(name, resting string, alsoAllowed flow.Offers) flow.State {
	return flow.State{
		Name:  name,
		Title: "Mail to read",
		About: "The agent has unread mail. Unread mail means it is NOT DONE, and an agent that is " +
			"not done is never prepared for new work. The mail is served with the directive; reading " +
			"it is the exit.",
		Says: says.Mail,
		Events: flow.Events{
			{On: cond.MailRead, To: resting, Why: "the mailbox is empty again"},
		},
		Verbs: append(flow.Offers{
			{Verb: verb.Mail, Why: "read your mailbox"},
			{Verb: verb.Log, Why: "record a note"},
		}, alsoAllowed...),
	}
}

// Escalated is a stop on a question only the user can answer. back is where the answer returns the
// agent to. The verb list is the role's own — what an escalation HOLDS BACK is the landing verbs of
// that role, and those differ; reading, tidying and telling the user anything never do.
func Escalated(name, back string, alsoAllowed flow.Offers) flow.State {
	return flow.State{
		Name:  name,
		Title: "Escalated",
		About: "The agent stopped on a question only the user can answer, and the question is " +
			"durable: one that evaporates leaves the agent silently stuck. It is repeated on every " +
			"ask, since an agent relaunched mid-escalation remembers nothing of how it got here.",
		Says: says.Escalated,
		Events: flow.Events{
			{On: cond.Resolved, To: back, Why: "the user answered and it cleared the escalation"},
		},
		Verbs: append(flow.Offers{
			{Verb: verb.Resume, Why: "clear the escalation once you have the answer"},
			{Verb: verb.Escalate, Why: "replace the question with a sharper one"},
			{Verb: verb.Mail, Why: "read your mailbox"},
			{Verb: verb.Log, Why: "record a note"},
			{Verb: verb.Meeting, Why: "say something in the meeting room"},
		}, alsoAllowed...),
	}
}

// Retired is wound down by a human: no further work is assigned. resting is where being brought back
// returns it to.
func Retired(name, resting string, alsoAllowed flow.Offers) flow.State {
	return flow.State{
		Name:  name,
		Title: "Retired",
		About: "A human wound this agent down: nothing further is assigned to it. Whatever it held " +
			"it has already finished — retirement is no NEW work, never abandoning what is in hand. " +
			"Un-retiring pushes a message, so waiting quietly is a kept promise.",
		Says: says.Retired,
		Events: flow.Events{
			{On: cond.BackInService, To: resting, Why: "a human brought it back"},
		},
		Verbs: append(flow.Offers{
			{Verb: verb.Mail, Why: "read your mailbox"},
			{Verb: verb.Task, Why: "read the backlog"},
			{Verb: verb.Log, Why: "record a note"},
		}, alsoAllowed...),
	}
}
