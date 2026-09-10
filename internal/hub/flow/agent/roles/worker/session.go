// package: hub/flow/agent/roles/worker / session
// type:    logic (the worker's session states, declared)
// job:     the three things the hub does to a worker's session before handing it work — clear it,
// switch the model under it — each a state of its own, because an event can arrive in
// the middle of any of them.
// limits:  the map. Each role declares its own, since a clear leads somewhere different for each.
package worker

import (
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/agent/says"
)

// clearing: the session is being discarded before work arrives.
var clearing = flow.State{
	Name:   Clearing,
	Title:  "Having its session cleared",
	Action: act.Clear,
	About: "A clear was typed into the session and the hub is waiting for the reading to fall — the " +
		"clear having HAPPENED, where a sleep only assumes it. Work arrives whole behind it, so the " +
		"previous unit is context to drop rather than condense.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Assigning, "the session is empty; work can be handed over"},
		{act.Failed, Assigning, "the clear never landed — a crowded session beats none at all"},
		{flow.Orphaned{}, Idle, "the hub restarted mid-clear"},
		{cond.SessionGone, Idle, "the pod went away mid-clear"},
	},
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}},
}

// retiering: the model under the agent is changed for the work coming.
var retiering = flow.State{
	Name:   Retiering,
	Title:  "Having its model changed",
	Action: act.Retier,
	About: "The work being handed over is rated for a different tier, so the model under the agent " +
		"is switched — which narrates and restarts the session on its way through. Nothing said into " +
		"this session survives it.",
	Says: says.Preparing,
	Events: flow.Events{
		{act.Done, Assigning, "the model is switched; work can be handed over"},
		{act.Failed, Assigning, "the switch never landed — hand the work over on the old model"},
		{flow.Orphaned{}, Idle, "the hub restarted mid-switch"},
	},
	Verbs: flow.Offers{{verb.Log, flow.Stay, "record a note"}},
}

// escalated: stopped on a decision only the user can make.
var escalated = flow.State{
	Name:  Escalated,
	Title: "Escalated",
	About: "The worker stopped on a question only the user can answer, and the question is durable: " +
		"one that evaporates leaves the agent silently stuck. It is repeated on every ask, since an " +
		"agent relaunched mid-escalation remembers nothing of how it got here.",
	Says: says.Escalated,
	Events: flow.Events{
		{cond.Resolved, Working, "the user answered and it cleared the escalation"},
	},
	// Only the LANDING verbs are held. An escalation stops an agent committing the fleet to
	// something while a question is open; it does not stop it reading, tidying its branch, or
	// telling the user anything.
	Verbs: flow.Offers{
		{verb.Resume, Working, "clear the escalation once you have the answer"},
		{verb.Escalate, flow.Stay, "replace the question with a sharper one"},
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Comment, flow.Stay, "comment on the task"},
		{verb.Log, flow.Stay, "record a note"},
		{verb.Fyi, flow.Stay, "one note to the user"},
		{verb.Git, flow.Stay, "read your changes"},
		{verb.Run, flow.Stay, "queue a slow build or test"},
		{verb.Rebase, flow.Stay, "align onto the reference branch"},
		{verb.Resolve, flow.Stay, "check your branch still merges"},
		{verb.Revoke, flow.Stay, "withdraw a pull request you have out"},
		{verb.Scratch, flow.Stay, "check work out into a disposable workspace"},
		{verb.Meeting, flow.Stay, "say something in the meeting room"},
	},
}

// retired: wound down by a human, and told so.
var retired = flow.State{
	Name:  Retired,
	Title: "Retired",
	About: "A human wound this worker down: no further work is assigned to it. Whatever it held it " +
		"has already finished. Un-retiring pushes a message, so waiting quietly is a kept promise.",
	Says: says.Retired,
	Events: flow.Events{
		{cond.BackInService, Idle, "a human brought it back"},
	},
	Verbs: flow.Offers{
		{verb.Mail, flow.Stay, "read your mailbox"},
		{verb.Task, flow.Stay, "read the backlog"},
		{verb.Log, flow.Stay, "record a note"},
	},
}

// Flow is the worker's whole map, in the order a reader should meet it.
var Flow = []flow.State{
	idle, assigning, working, refreshing, reworking, submitting, gating, submitted, resolving,
	between, picking, featureGated, featureDone, releasing, yielding, promoting, rebasing,
	clearing, retiering, escalated, retired, mail, stalled,
}

// Start is where a worker with no state stored begins.
const Start = Idle
