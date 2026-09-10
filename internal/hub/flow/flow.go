// package: hub/flow / flow
// type:    logic (the vocabulary an agent's flow is written in)
// job:     the world every condition reads, and the names a flow file spells its declarations with.
// The four role flows are written against this and nothing else.
// limits:  the vocabulary. The engine is flow/machine's, the conditions are flow/cond's, and what an
// action DOES is the acting half's (-> the *_act.go files beside each map).
package flow

import (
	"time"

	"github.com/flo-at/sindri/internal/hub/flow/machine"
	"github.com/flo-at/sindri/internal/hub/situation"
	"github.com/flo-at/sindri/internal/hub/store"
)

// State, Events and the rest are the engine's shapes bound to an agent's world, so a flow file names
// them without a type parameter in sight.
type (
	State      = machine.State[World]
	Transition = machine.Transition[World]
	Condition  = machine.Condition[World]
	Action     = machine.Action
	Outcome    = machine.Outcome
	Offer      = machine.Offer
	Verb       = machine.Verb
	Topic      = machine.Topic
	Events     = []machine.Transition[World]
	Offers     = []machine.Offer
)

// Stay is the target of a verb that leaves the agent where it is.
const Stay = machine.Stay

// Orphaned is the built-in exit every acting state needs: this hub never started that action, so a
// previous one died holding it.
type Orphaned = machine.Orphaned

// How stale an answer may be. Three of them, named for what waits on the answer rather than for a
// duration, so a condition declares WHY it is watched closely and not just how often.
const (
	// Blocking: an agent is stopped until this is noticed.
	Blocking = 10 * time.Second
	// Soon: work sits idle until this is noticed.
	Soon = time.Minute
	// Eventually: nothing waits on it. Its topic normally carries it; the poll is only the backstop.
	Eventually = 5 * time.Minute
)

// DefaultEvery is the cadence of a state whose every exit arrives by topic or outcome, so nothing
// obliges a poll at all.
const DefaultEvery = Eventually

// World is everything an agent's conditions may read, gathered ONCE per pass. Conditions reaching
// for what they need separately can decide from a world that never existed.
type World struct {
	// Where the agent stands: its roster row, its state row, the observer's last reading, and what
	// its project could hand it.
	situation.Situation

	// Held is the verdict on the work IN HAND, Awaiting the one on a PR still to land while the
	// agent holds nothing else.
	Held     Verdict
	Awaiting Verdict

	// Aim and Ceiling are the comment budget the work directives quote.
	Aim, Ceiling float64

	// Subtasks are the held feature's open children, Gated the ones still awaiting approval.
	Subtasks []store.Task
	Gated    []store.Task

	// Next is what the backlog would hand this agent, HasNext false when there is nothing, and
	// NextIsFeature whether taking it puts the agent in the subtask loop rather than on a leaf.
	Next          store.Task
	HasNext       bool
	NextIsFeature bool

	// Unread is how many messages the agent has not read. Unread mail means it is NOT DONE, which is
	// what keeps a clear from landing on top of something it has never seen.
	Unread int

	// Conflicted: the merge of a PR this agent filed hit a conflict, so the branch is back in its
	// workspace with the resolution to do.
	Conflicted bool
	// GainedChildren: the leaf task it holds now has open children, so what it holds is a feature.
	GainedChildren bool
	// MilestoneLanded: an interim PR of its own merged and it still holds the work — its standing
	// branch is behind the base that merge moved.
	MilestoneLanded bool

	// ReviewWaiting: a pull request is filed with no reviewer on it. A reviewer's equivalent of the
	// backlog having something, and the only reason a free one leaves idle.
	ReviewWaiting bool

	// SplitTree: another agent is working inside the tree this one holds the container of.
	SplitTree bool
	// TaskGone: the work it holds is no longer in the backlog at all.
	TaskGone bool
}

// Verdict is a rejection standing against one piece of work: what the reviewer said, and how many
// rounds it has taken. Round is what should change an author's approach, so it travels with it.
type Verdict struct {
	Rejected bool
	Feedback string
	Round    int
}

// Empty names the states that mean HOLDING NOTHING. Entering one releases the work: an agent left
// resting with a task still on its row is one the board shows as free and the assigner refuses,
// which is the disagreement this closes.
var Empty = map[string]bool{
	"worker/idle": true, "reviewer/idle": true, "planner/idle": true,
}

// Releases reports a state whose meaning is that the agent holds nothing.
func Releases(state string) bool { return Empty[state] }
