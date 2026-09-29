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
	"github.com/flo-at/sindri/internal/hub/world/situation"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// State, Events and the rest are the engine's shapes bound to an agent's world, so a flow file names
// them without a type parameter in sight.
type (
	State      = machine.State[World]
	Group      = machine.Group[World]
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

// Kind is what an exit is in the flow (-> machine.Kind), spelled as a flow file spells the rest.
type Kind = machine.Kind

const (
	Progress     = machine.Progress
	Setback      = machine.Setback
	Fault        = machine.Fault
	Intervention = machine.Intervention
	Upkeep       = machine.Upkeep
	WorldMoved   = machine.WorldMoved
)

// Stay is the target of a verb that leaves the agent where it is.
const Stay = machine.Stay

// What a still screen means, spelled the way a flow file spells everything else it declares.
type WhenIdle = machine.WhenIdle

const (
	// LetItRest: the agent waits on somebody else here, so a screen that has stopped is correct.
	LetItRest = machine.LetItRest
	// Nudge: the agent is the one expected to move, so a screen that has stopped is a fault.
	Nudge = machine.Nudge
)

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

// The two dwells the pod's own states are bounded by, both named here beside the staleness constants
// because they belong to the same vocabulary: how long a thing may stand before it stops being that
// thing.
const (
	// IdleStopThreshold is how long an agent may hold nothing before the hub reclaims its pod. A stop
	// preserves the session, so being wrong costs only the next start's latency.
	IdleStopThreshold = 30 * time.Minute
	// LaunchBound is how long a launch may stand before it stops reading as one still on its way. Wide
	// enough for a cold pod boot plus the entrypoint starting tmux.
	LaunchBound = 2 * time.Minute
)

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

	// SubmitAsked: the author asked for a submit and the hub has not yet taken it. A request rather
	// than a flag, since an author may ask again on a tree it has moved on since.
	SubmitAsked bool
	// InterviewQuestion is the submit question standing unanswered, "" when none is. Read here so
	// the words an author is told where it stands are the question itself.
	InterviewQuestion string

	// GateRefused: the last gate this agent opened to land something answered without landing it.
	// Nothing was filed, so there is no pull request carrying a verdict to read instead.
	GateRefused bool

	// ReviewWaiting: a pull request is filed with no reviewer on it. A reviewer's equivalent of the
	// backlog having something, and the only reason a free one leaves idle.
	ReviewWaiting bool

	// SplitTree: another agent is working inside the tree this one holds the container of.
	SplitTree bool
	// TaskGone: the work it holds is no longer in the backlog at all.
	TaskGone bool

	// Wanted: its OWN kind of work is waiting unclaimed — a rated task for a worker, a filed review
	// for a reviewer. One role's queue never wakes another's, which is why this is folded per role
	// rather than left to each condition to work out.
	Wanted bool
	// PoolCovered: another agent of this role is up and empty-handed, so it would take the waiting
	// work itself. Nothing asleep needs bringing back for work somebody awake will claim.
	PoolCovered bool
	// FirstAsleep: of the stopped agents of this role that could take the waiting work, this is the
	// one to bring back. A fleet-wide choice made per agent, so one wake starts one pod.
	FirstAsleep bool
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
