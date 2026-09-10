// package: hub/flow/machine / state
// type:    logic (what a state is, declared)
// job:     the vocabulary a flow is written in — a state, the action it runs, everything that can
// move a subject out of it, the verbs it offers, and the words it answers with. Declared as data,
// so a flow file reads as the map of how that kind of subject works.
// limits:  the shapes. What any of them MEAN is the domain's, and running them is loop.go's.
package machine

import (
	"context"
	"time"
)

// Topic is a named event that makes a condition worth checking early. It may only SHORTEN latency —
// every transition stays Reachable by the state's own poll — so a dropped one costs a beat.
type Topic string

// Event is something that moves a subject out of a state — an Outcome of the state's own action, or
// a Condition about the world. The map does not distinguish them; the engine does.
type Event interface{ EventName() string }

// Outcome is how a state's action finished. The engine owns these: the action returns in the same
// pass that started it, so an outcome cannot be missed and is never polled for.
type Outcome struct{ Name string }

// EventName is how this outcome appears on the record.
func (o Outcome) EventName() string { return o.Name }

// Condition is a question about the world, evaluated on a poll and early on any of its Wake topics.
// Pure: it is handed a gathered world and nothing to write with.
//
// Within is how stale the answer may be — a property of the QUESTION, the same wherever it is asked,
// so a state inherits its cadence from what it watches instead of carrying a number somebody chose.
type Condition[W any] struct {
	Name   string
	Within time.Duration
	Holds  func(W) bool
	Wake   []Topic
}

// EventName is how this condition appears on the record.
func (c Condition[W]) EventName() string { return c.Name }

// OrphanCheck is how stale "was this action left behind by a hub that died" may be. It only ever
// matters just after a restart, so noticing it within a minute is prompt enough.
const OrphanCheck = time.Minute

// Orphaned holds when this machine never started this state's action and none is running: a hub that
// is gone left the subject here. It replaces a bespoke recovery path per in-flight state.
type Orphaned struct{}

// EventName is how an orphaned action appears on the record.
func (Orphaned) EventName() string { return "orphaned" }

// Transition is one way out of a state: what happened, where it leads, and the words for the record.
// To may be Stay, for something worth acting on that moves the subject nowhere.
type Transition[W any] struct {
	On  Event
	To  string
	Why string
}

// Action is what the hub does in a state — an IDENTITY, so a flow file names one without importing
// anything that writes. Its implementation registers against the name, or the build fails.
type Action struct {
	Name string
	// Outcomes is every way this action can finish. A state declaring it must handle each, and an
	// implementation returning one that is not here fails loudly. The FIRST is the expected one —
	// the path the action exists to take — which is what a prediction follows when it cannot run it
	// (-> Machine.Would).
	Outcomes []Outcome
}

// Doer performs one action. The only thing in a flow allowed to write anything.
type Doer[W any] func(ctx context.Context, w W) (Outcome, error)

// Verb is a command a subject may run, offered per state — so what is possible is read off where the
// subject stands rather than re-derived by every verb for itself.
type Verb struct {
	Name string
	Help string
}

// Stay is the target of a verb or event that moves nobody — a prod is the shape it exists for.
const Stay = ""

// Offer is one verb a state makes available, and where running it leads.
type Offer struct {
	Verb Verb
	To   string
	Why  string
}

// State is one state of a flow, DECLARED rather than coded around. The struct reads as that state's
// documentation: what the hub is doing here, everything that can move the subject out, what it may
// type, and what it is told if it asks.
type State[W any] struct {
	Name  string
	Title string
	About string
	// Action is what the hub does while the subject is here. Nil means the hub does nothing — the
	// subject is working, or waiting on somebody else.
	Action *Action
	// Says names what the subject is told when it asks where it stands. The words live with the
	// domain; this is the identity of them.
	Says string
	// Events is everything that moves the subject out: the action's outcomes and the world's
	// conditions, in one list, each with where it leads.
	Events []Transition[W]
	// Verbs is what the subject may run here. Anything else is refused by name, with this list.
	Verbs []Offer
}
