// package: hub/flow/run / cond
// type:    logic (the questions the run map asks about a run)
// job:     every condition the run flow watches, as a value it names — the question, how stale the
// answer may be, and the topics that carry it early. PURE: handed a gathered world, nothing else.
// limits:  the questions. Where each one leads is the map's (-> run.go).
package run

import (
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
)

// of builds a condition: its name, how stale the answer may be, the topics that carry it early, and
// the question itself. The tolerance belongs to the QUESTION, so every state watching it inherits
// its cadence from that rather than carrying a number somebody chose.
func of(name string, within time.Duration, wake []flow.Topic, holds func(World) bool) Condition {
	return Condition{Name: name, Within: within, Wake: wake, Holds: holds}
}

// AtFront: this run leads the fleet's one queue and nothing else is executing, so the slot is its
// own. Watched CLOSELY: a queue that does not advance is the whole fleet waiting on nothing.
var AtFront = of("at-front", flow.Blocking, []flow.Topic{topic.GateFinished},
	func(w World) bool { return w.Position == 1 && !w.SlotTaken })

// Stale: the agent that queued this has gone, or has moved on to different work since. Cheaper to
// notice than to materialize a worktree, build an image and run against a tree nobody wants tested.
var Stale = of("stale", flow.Soon, []flow.Topic{topic.TaskClosed, topic.TaskAvailable},
	func(w World) bool { return w.Stale != "" })

// Settled: the row itself says the run is over — finished on its own, or withdrawn by a human while
// it sat in the queue. Either way there is nothing left for the machine to do with it.
var Settled = of("settled", flow.Soon, nil, func(w World) bool { return !api.RunOpen(w.Run) })

// Conditions is every declared condition, for the check that each one is watched by some state.
var Conditions = []Condition{AtFront, Stale, Settled}
