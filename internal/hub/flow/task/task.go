// package: hub/flow/task / task
// type:    logic (a task's own flow, declared)
// job:     the map of a task's life — proposed, open, held, under review, closed — with the
// conditions that move it between them. A task is moved by OBSERVATION rather than by the hub doing
// things to it: its status is a claim about the world, and these are the checks on that claim.
// limits:  the map and the world it reads. Writing a status is the acting half's (-> task_act.go).
package task

import (
	"time"

	"github.com/flo-at/sindri/internal/hub/flow/machine"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
)

// The states a task can stand in. Each is the status it claims about itself, plus the approval gate
// ahead of all of them.
const (
	Proposed  = "task/proposed"
	Open      = "task/open"
	Held      = "task/held"
	Reviewing = "task/reviewing"
	Closed    = "task/closed"
)

// The machine's shapes bound to a task's world, so this file names them without a type parameter.
// The engine's shapes bound to a task's world, so this file names them without a type
// parameter in sight.
type (
	State      = machine.State[World]
	Condition  = machine.Condition[World]
	Events     = []machine.Transition[World]
	Transition = machine.Transition[World]
)

// World is everything a task's conditions read, gathered once per pass. It is the same set of facts
// the repair sweep weighed by hand, named rather than passed as a struct of four bools.
type World struct {
	ID      string
	Project string
	Exists  bool
	// Pending: the user has not yet ruled on this proposal, so nothing may claim it.
	Pending bool
	// Refused: the user rejected the proposal outright.
	Refused bool
	// Holder is the agent that holds it, "" when nobody does.
	Holder string
	// ActivePR: a pull request neither merged nor rejected — the task really is out for review.
	ActivePR bool
	// Landed: its work went in on a FINAL pull request. An interim contribution does not count; that
	// milestone is not the task's own end, and counting it as one closed trees still being worked.
	Landed bool
	// OpenChildren: work remains beneath it, whatever it claims about itself.
	OpenChildren bool
}

// of builds a condition: how stale the answer may be, the topics that carry it early, the question.
func of(name string, within time.Duration, wake []machine.Topic, holds func(World) bool) Condition {
	return Condition{Name: name, Within: within, Wake: wake, Holds: holds}
}

// How stale an answer about a task may be. A task is not blocked on anything the way an agent is, so
// none of these is urgent; the topics carry the ones that matter.
const (
	soon       = time.Minute
	eventually = 10 * time.Minute
)

var (
	// approved: the user ruled, so the proposal is a task like any other.
	approved = of("approved", soon, []machine.Topic{topic.TaskApproved},
		func(w World) bool { return !w.Pending && !w.Refused })
	// refused: the user rejected it, and a refused proposal is not work anybody will do.
	refused = of("refused", soon, []machine.Topic{topic.TaskApproved}, func(w World) bool { return w.Refused })
	// claimed: an agent holds it.
	claimed = of("claimed", soon, []machine.Topic{topic.TaskAvailable}, func(w World) bool { return w.Holder != "" })
	// unheld: nobody holds it. THE stale claim the repair sweep existed for — a task reading
	// in_progress with no agent behind it is a task nobody is doing.
	unheld = of("unheld", soon, []machine.Topic{topic.TaskAvailable}, func(w World) bool { return w.Holder == "" })
	// outForReview: a live pull request stands against it.
	outForReview = of("out-for-review", soon, []machine.Topic{topic.PRVerdict}, func(w World) bool { return w.ActivePR })
	// noPR: nothing is out. A task reading in_review with no PR is the sweep's other stale claim.
	noPR = of("no-pr", soon, []machine.Topic{topic.PRVerdict, topic.PRMerged}, func(w World) bool { return !w.ActivePR })
	// landed: its work went in and nothing is left beneath it. A merge is the end of a task, so a
	// tree left open by one that took the wrong path is closed here rather than waiting to be noticed.
	landed = of("landed", soon, []machine.Topic{topic.PRMerged},
		func(w World) bool { return w.Landed && !w.OpenChildren })
	// childrenOpen: work remains beneath it, which outranks any claim that it is finished.
	childrenOpen = of("children-open", soon, []machine.Topic{topic.TaskClosed}, func(w World) bool { return w.OpenChildren })
	// gone: the task is no longer at its source at all.
	gone = of("gone", eventually, []machine.Topic{topic.TaskClosed}, func(w World) bool { return !w.Exists })
)

// proposed: the user has not ruled on it, so it is not work yet.
var proposed = State{
	Name:  Proposed,
	Title: "Proposed, awaiting the user",
	About: "A planner proposed this and the user has not ruled. Nothing may claim it: an approval " +
		"gate a worker can walk around is not a gate.",
	Events: Events{
		{gone, Closed, "it is no longer at its source"},
		{refused, Closed, "the user rejected the proposal"},
		{approved, Open, "the user approved it, so it is claimable like anything else"},
	},
}

// open: claimable, and claimed by nobody.
var open = State{
	Name:  Open,
	Title: "Open",
	About: "Nobody holds it and nothing is out for it. This is what the assigner draws from, and " +
		"where a task returns whenever a claim on it turns out not to be real.",
	Events: Events{
		{gone, Closed, "it is no longer at its source"},
		{landed, Closed, "its work went in and nothing is left beneath it"},
		{claimed, Held, "an agent took it"},
	},
}

// held: an agent has it in hand.
var held = State{
	Name:  Held,
	Title: "Held by an agent",
	About: "An agent holds this and is working it. The claim is checked rather than trusted: a task " +
		"reading in_progress with no agent behind it was the commonest stale row the repair sweep " +
		"existed to fix, and it is an ordinary transition here.",
	Events: Events{
		{gone, Closed, "it is no longer at its source"},
		{landed, Closed, "its work went in"},
		{outForReview, Reviewing, "a pull request was filed for it"},
		{unheld, Open, "the agent that held it does not hold it any more"},
	},
}

// reviewing: a pull request stands against it.
var reviewing = State{
	Name:  Reviewing,
	Title: "Out for review",
	About: "A live pull request stands against this task. When none does, the task goes back to " +
		"whoever holds it — or to the backlog if nobody does, which is the second stale claim the " +
		"sweep was written for.",
	Events: Events{
		{gone, Closed, "it is no longer at its source"},
		{landed, Closed, "the pull request merged and nothing is left beneath it"},
		{noPR, Held, "nothing is out for it any more, and its holder still has it"},
	},
}

// closed: finished, or abandoned — unless the tree beneath it says otherwise.
var closed = State{
	Name:  Closed,
	Title: "Closed",
	About: "Finished, scrapped, or merged. Open children outrank that: work beneath a closed parent " +
		"is real and unfinished, so the parent is REOPENED rather than left lying — closing one over " +
		"open children is the incident this whole check traces back to.",
	Events: Events{
		{childrenOpen, Open, "work beneath it is open, so it was not finished after all"},
	},
}

// Flow is a task's whole map.
var Flow = []State{proposed, open, held, reviewing, closed}

// Start is where a task with no state of its own begins.
const Start = Open

// Conditions is every declared condition, for the check that each is watched.
var Conditions = []Condition{approved, refused, claimed, unheld, outForReview, noPR, landed, childrenOpen, gone}
