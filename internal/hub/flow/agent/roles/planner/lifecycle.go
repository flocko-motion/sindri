// package: hub/flow/agent/roles/planner / lifecycle
// type:    logic (the planner's session and pod lifecycle, declared)
// job:     name where each shared lifecycle state leads for a planner, and build it through the one
// factory that owns what can move a subject out of one.
// limits:  the destinations. The event lists are lifecycle's.
package planner

import (
	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/roles/lifecycle"
)

const (
	Launching = "planner" + lifecycle.LaunchingIn
	Stopping  = "planner" + lifecycle.StoppingIn
	Clearing  = "planner" + lifecycle.ClearingIn
	Escalated = "planner" + lifecycle.EscalatedIn
	Retired   = "planner" + lifecycle.RetiredIn
)

var (
	launching = lifecycle.In(GroupPod, lifecycle.Launching(Launching, Idle))
	stopping  = lifecycle.In(GroupPod, lifecycle.Stopping(Stopping, Idle))
)

// A planner's clear leads nowhere but back to rest: nothing is handed to it, so there is no
// hand-over waiting behind the empty session.
var clearing = lifecycle.In(GroupPod, lifecycle.Clearing(Clearing, Idle, Idle))

// The verbs an escalated planner keeps: everything but shipping a proposal. Reading, proposing tasks
// and talking to the user are how a planner gets its answer in the first place.
var escalated = lifecycle.In(GroupPod, lifecycle.Escalated(Escalated, Planning, flow.Offers{
	{Verb: verb.Git, Why: "read the changes"},
	{Verb: verb.Revoke, Why: "withdraw a proposal you have out"},
	{Verb: verb.Task, Why: "read the backlog"},
	{Verb: verb.CreateTask, Why: "propose a task"},
	{Verb: verb.Comment, Why: "comment on a task"},
	{Verb: verb.Fyi, Why: "one note to the user"},
	{Verb: verb.Rebase, Why: "align onto the reference branch"},
}))

var retired = lifecycle.In(GroupPod, lifecycle.Retired(Retired, Idle, nil))
