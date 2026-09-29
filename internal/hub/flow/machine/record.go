// package: hub/flow/machine / record
// type:    logic (what one pass says about itself)
// job:     the steps a pass can report, the entry each carries, and the port the domain implements
// to receive them. One correlation id runs through every entry of one pass.
// limits:  the shape. Where the record is kept, and for how long, is the domain's.
package machine

import (
	"fmt"
	"sync/atomic"
)

// Step is what a pass just did.
type Step string

const (
	StepLooked    Step = "looked"    // a pass began, and why
	StepStarted   Step = "started"   // the state's action began, holding its own ctx
	StepOutcome   Step = "outcome"   // the action finished this way
	StepCancelled Step = "cancelled" // the action was abandoned, with the reason
	StepMoved     Step = "moved"     // the subject changed state, and what moved it
	StepVerb      Step = "verb"      // a verb was run, or refused
	StepFailed    Step = "failed"    // gather, dispatch or action failed
)

// Entry is one line of a pass's record. Pass ties every line of one pass together: what prompted it,
// the state it looked at, the action it ran, and where the subject ended up.
type Entry struct {
	Pass    string
	Subject string
	State   string
	Step    Step
	Detail  string
	Err     error
}

// Recorder receives the machine's account of its own passes — including an action started and then
// abandoned, which a domain logging its own work cannot see and the machine can.
//
// Called from the loop and from an action's own goroutine, so it must be safe under concurrent
// calls, and it must never call back into the machine.
type Recorder interface {
	Record(Entry)
}

// counter mints correlation ids.
type counter struct{ n atomic.Uint64 }

func (c *counter) next() string { return fmt.Sprintf("p-%06x", c.n.Add(1)) }

// record hands one entry to the domain's recorder, if it declared one.
func (m *machine[W]) record(e Entry) {
	if m.cfg.Record != nil {
		m.cfg.Record.Record(e)
	}
}
