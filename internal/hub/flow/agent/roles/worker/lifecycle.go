// package: hub/flow/agent/roles/worker / lifecycle
// type:    logic (the worker's session and pod lifecycle, declared)
// job:     name where each shared lifecycle state leads for a worker, and build it through the one
// factory that owns what can move a subject out of one.
// limits:  the destinations. The event lists are lifecycle's, and a role omitting one does not
// compile.
package worker

import (
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/lifecycle"
)

// The lifecycle states, spelled off the one suffix each carries.
const (
	Launching = "worker" + lifecycle.LaunchingIn
	Stopping  = "worker" + lifecycle.StoppingIn
	Clearing  = "worker" + lifecycle.ClearingIn
	Mail      = "worker" + lifecycle.MailIn
	Escalated = "worker" + lifecycle.EscalatedIn
	Retired   = "worker" + lifecycle.RetiredIn
)

// The pod's two states. Both rest at Idle, which re-decides the moment the pod's own reading moves —
// so a worker started for waiting work claims it from there, through the ordinary route.
var (
	launching = lifecycle.Launching(Launching, Idle)
	stopping  = lifecycle.Stopping(Stopping, Idle)
)

// clearing leads into the hand-over it was taken for, and back to idle when it could not happen.
var clearing = lifecycle.Clearing(Clearing, Assigning, Idle)

var mail = lifecycle.Mail(Mail, Idle, flow.Offers{
	{Verb: verb.Task, Why: "read the backlog"},
	{Verb: verb.Escalate, Why: "stop on a question"},
})

// The verbs an escalated worker keeps: everything but the LANDING ones. An escalation stops an agent
// committing the fleet to something while a question is open; it does not stop it reading, tidying
// its branch, or telling the user anything.
var escalated = lifecycle.Escalated(Escalated, Working, flow.Offers{
	{Verb: verb.Task, Why: "read the backlog"},
	{Verb: verb.Comment, Why: "comment on the task"},
	{Verb: verb.Fyi, Why: "one note to the user"},
	{Verb: verb.Git, Why: "read your changes"},
	{Verb: verb.Run, Why: "queue a slow build or test"},
	{Verb: verb.Rebase, Why: "align onto the reference branch"},
	{Verb: verb.Resolve, Why: "check your branch still merges"},
	{Verb: verb.Revoke, Why: "withdraw a pull request you have out"},
	{Verb: verb.Scratch, Why: "check work out into a disposable workspace"},
})

var retired = lifecycle.Retired(Retired, Idle, nil)
