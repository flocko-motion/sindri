// package: hub/flow/agent/roles/reviewer / lifecycle
// type:    logic (the reviewer's session and pod lifecycle, declared)
// job:     name where each shared lifecycle state leads for a reviewer, and build it through the one
// factory that owns what can move a subject out of one.
// limits:  the destinations. The event lists are lifecycle's, and a role omitting one does not
// compile — which is how the session-gone edge this map lacked came to strand a reviewer.
package reviewer

import (
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/lifecycle"
)

const (
	Launching = "reviewer" + lifecycle.LaunchingIn
	Stopping  = "reviewer" + lifecycle.StoppingIn
	Clearing  = "reviewer" + lifecycle.ClearingIn
	Mail      = "reviewer" + lifecycle.MailIn
	Escalated = "reviewer" + lifecycle.EscalatedIn
	Retired   = "reviewer" + lifecycle.RetiredIn
)

var (
	launching = lifecycle.Launching(Launching, Idle)
	stopping  = lifecycle.Stopping(Stopping, Idle)
)

// clearing leads into the hand-over: a review is read whole, so the session is discarded before the
// branch arrives rather than by whoever hands it over.
var clearing = lifecycle.Clearing(Clearing, Taking, Idle)

var mail = lifecycle.Mail(Mail, Idle, flow.Offers{
	{Verb: verb.Escalate, Why: "stop on a question"},
})

// The verbs an escalated reviewer keeps: everything but the verdict. One stopped on a question still
// reads the diff and the task, and still says what it has found.
var escalated = lifecycle.Escalated(Escalated, Reviewing, flow.Offers{
	{Verb: verb.Task, Why: "read the backlog"},
	{Verb: verb.Comment, Why: "comment on the task"},
	{Verb: verb.Fyi, Why: "one note to the user"},
	{Verb: verb.Run, Why: "queue a slow build or test"},
	{Verb: verb.Git, Why: "read the diff"},
})

var retired = lifecycle.Retired(Retired, Idle, nil)
