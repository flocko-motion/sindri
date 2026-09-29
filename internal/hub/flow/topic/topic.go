// package: hub/flow/topic / topic
// type:    logic (the named events that wake a condition early)
// job:     the closed set of topics one part of the hub publishes and another's states listen for —
// so cross-module coupling is greppable in both directions instead of implied by a bare notify.
// limits:  the names. Publishing is the writer's, listening is a condition's (-> flow/cond).
package topic

import "github.com/flo-at/sindri/internal/hub/flow/machine"

// A topic may only SHORTEN latency. Every transition it accelerates must also be reachable by its
// state's own poll, so a dropped topic costs a beat and never a stuck agent.
const (
	TaskAvailable  machine.Topic = "task-available"  // the backlog gained something claimable
	TaskClosed     machine.Topic = "task-closed"     // a task was closed, scrapped or reparented
	TaskApproved   machine.Topic = "task-approved"   // the approval gate opened on a proposal
	PRVerdict      machine.Topic = "pr-verdict"      // a PR was approved, rejected or revoked
	PRMerged       machine.Topic = "pr-merged"       // a PR landed on its base
	ReviewFiled    machine.Topic = "review-filed"    // a PR is waiting for a reviewer to take it
	GateFinished   machine.Topic = "gate-finished"   // the quality gate answered
	MailArrived    machine.Topic = "mail-arrived"    // an agent has unread mail
	HoldingChanged machine.Topic = "holding-changed" // what an agent holds was handed over, or taken away
	AgentParked    machine.Topic = "agent-parked"    // retired, stopped, or a clear armed
	SessionRead    machine.Topic = "session-read"    // the observer took a fresh look at a pane
	SubmitAsked    machine.Topic = "submit-asked"    // an author asked for a submit, or answered for one
)

// All is every topic, for the check that each is both published and listened for.
var All = []machine.Topic{
	TaskAvailable, TaskClosed, TaskApproved, PRVerdict, PRMerged, ReviewFiled,
	GateFinished, MailArrived, HoldingChanged, AgentParked, SessionRead, SubmitAsked,
}
