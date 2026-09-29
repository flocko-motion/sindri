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
// every transition stays reachable by the state's own poll — so a dropped one costs a beat.
type Topic string

// Event is something that moves a subject out of a state — an Outcome of the state's own action, or
// a Condition about the world. The map does not distinguish them; the engine does.
type Event interface{ EventName() string }

// Kind says what an exit IS in the flow, apart from what triggers it. The engine runs every kind
// alike; the debug view colours and hides by it.
type Kind string

const (
	Progress     Kind = "progress"     // a step along the intended path
	Setback      Kind = "setback"      // an intended step back: the flow working as designed
	Fault        Kind = "fault"        // a runtime exception, and the recovery from it
	Intervention Kind = "intervention" // a human stepped in
	Upkeep       Kind = "upkeep"       // the hub looking after the pod and session
	WorldMoved   Kind = "world-moved"  // the work changed or vanished underneath, through nobody's fault
)

// Outcome is how a state's action finished. The engine owns these: the action returns in the same
// pass that started it, so an outcome cannot be missed and is never polled for.
type Outcome struct {
	Name string
	Kind Kind
}

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
	Kind   Kind
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

// KindOf is what this transition is in the flow: its event's kind. Orphaned is always a fault — only
// a dead hub's leftover fires it.
func (t Transition[W]) KindOf() Kind {
	switch e := t.On.(type) {
	case Condition[W]:
		return e.Kind
	case Outcome:
		return e.Kind
	case Orphaned:
		return Fault
	}
	return ""
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
	// Awaits marks an action that waits on the SUBJECT — an exchange with it, rather than work done
	// upon it. It never runs on a caller's goroutine and nobody holding the line waits behind one:
	// the answer such a caller would be waiting for is the call it is inside, so waiting there is a
	// deadlock and not a delay.
	Awaits bool
}

// Doer performs one action. The only thing in a flow allowed to write anything.
type Doer[W any] func(ctx context.Context, w W) (Outcome, error)

// Verb is a command a subject may run, offered per state — so what is possible is read off where the
// subject stands rather than re-derived by every verb for itself.
type Verb struct {
	Name string
	Help string
}

// Stay is the target of an event that moves nobody — a prod is the shape it exists for.
const Stay = ""

// Offer is one verb a state makes available, and why it is there. It declares NO destination:
// running a verb moves nobody, and where the subject then stands is whatever its conditions
// conclude from the world that verb changed (-> Machine.Would). A declared landing was a second
// account of one truth, and it was wrong for its entire life without anyone noticing.
type Offer struct {
	Verb Verb
	Why  string
}

// WhenIdle says what a still screen MEANS where the subject stands — correct in one state, a fault
// in the next. The zero value is neither, so a state that never considered it is refused.
type WhenIdle uint8

const (
	IdleUndeclared WhenIdle = iota
	// LetItRest: it waits on somebody else here — a verdict, a queue, a human, work not yet arrived.
	LetItRest
	// Nudge: it is the one expected to move, so a screen that has stopped is a fault worth prodding.
	Nudge
)

// Group is a region whose exits every state inside inherits, so none can forget one. First is checked
// before a state's own, Then after; only conditions, since an outcome belongs to one state's action.
type Group[W any] struct {
	Name  string
	Title string
	About string
	// In is the group this one sits inside, "" for a top-level one.
	In    string
	First []Transition[W]
	Then  []Transition[W]
}

// State is one state of a flow, DECLARED rather than coded around. The struct reads as that state's
// documentation: what the hub is doing here, everything that can move the subject out, what it may
// type, and what it is told if it asks.
type State[W any] struct {
	Name  string
	Title string
	About string
	// In is the group this state sits inside, whose exits it inherits (-> Group); "" for none.
	In string
	// Action is what the hub does while the subject is here. Nil means the hub does nothing — the
	// subject is working, or waiting on somebody else.
	Action *Action
	// Says names what the subject is told when it asks where it stands. The words live with the
	// domain; this is the identity of them.
	Says string
	// Tells sends Says to the subject UNPROMPTED the moment it lands here. For a state where the
	// SUBJECT is the actor and no action spoke on the way in: the alternative is asking it to ask,
	// a round trip whose answer the hub already holds.
	Tells bool
	// NudgeAfter is how long a still screen is tolerated before the prod, zero taking the flow's own
	// dwell — longer where a pause is ordinary, so nothing nudges an agent that is merely thinking.
	WhenIdle   WhenIdle
	NudgeAfter time.Duration
	// Events is everything that moves the subject out: the action's outcomes and the world's
	// conditions, in one list, each with where it leads.
	Events []Transition[W]
	// Verbs is what the subject may run here. Anything else is refused by name, with this list.
	Verbs []Offer
}
