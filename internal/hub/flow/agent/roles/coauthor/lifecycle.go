// package: hub/flow/agent/roles/coauthor / lifecycle
// type:    logic (the coauthor's session and pod lifecycle, declared)
// job:     name where each shared lifecycle state leads for a coauthor, and build it through the one
// factory that owns what can move a subject out of one.
// limits:  the destinations. The event lists are lifecycle's.
package coauthor

import (
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/lifecycle"
)

const (
	Launching = "coauthor" + lifecycle.LaunchingIn
	Stopping  = "coauthor" + lifecycle.StoppingIn
	Clearing  = "coauthor" + lifecycle.ClearingIn
	Escalated = "coauthor" + lifecycle.EscalatedIn
	Retired   = "coauthor" + lifecycle.RetiredIn
)

// A coauthor's pod is reclaimed only when a human asks: the surface refuses to take back a session
// somebody is sitting in, so the idle rule never fires here and the state exists for the ask alone.
var (
	launching = lifecycle.In(GroupPod, lifecycle.Launching(Launching, Collab))
	stopping  = lifecycle.In(GroupPod, lifecycle.Stopping(Stopping, Collab))
)

// Its clear leads back to the seat it shares: there is no queue behind it to hand anything over.
var clearing = lifecycle.In(GroupPod, lifecycle.Clearing(Clearing, Collab, Collab))

// A coauthor lands nothing through the hub — it commits with git itself — so an escalation shuts
// nothing but leaves the question on the record for the user sitting with it.
var escalated = lifecycle.In(GroupPod, lifecycle.Escalated(Escalated, Collab, flow.Offers{
	{Verb: verb.Task, Why: "read the backlog"},
	{Verb: verb.CreateTask, Why: "propose a task"},
	{Verb: verb.Comment, Why: "comment on a task"},
	{Verb: verb.Scratch, Why: "check work out into a disposable workspace"},
	{Verb: verb.Git, Why: "read the changes"},
	{Verb: verb.Run, Why: "queue a slow build or test"},
}))

var retired = lifecycle.In(GroupPod, lifecycle.Retired(Retired, Collab, nil))
