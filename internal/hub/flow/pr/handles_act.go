// package: hub/flow/pr / handles_act
// type:    assembly (the acting half of a pull request's flow)
// job:     hold what acting on a merge intent needs — the hub's handles — so the map beside this
// stays a declaration and everything that WRITES is on one type.
// limits:  acting. What a merge intent's states mean is pr.go's, and this decides nothing.
package pr

import (
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/flow/run"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
)

// Act performs what a pull request's flow decides.
type Act struct{ *core.Core }

// New builds the acting half over the hub's handles.
func New(c *core.Core) *Act { return &Act{Core: c} }

// announceHolding says what an agent holds has changed, so its own map re-decides where that leaves
// it. A HINT, like every topic: the row is read again on the agent's own beat regardless.
func (a *Act) announceHolding(project, agent string) {
	a.Deps.Notify()
	a.Flow.Wake(project, agent, topic.HoldingChanged)
}

// announceVerdict says something standing against a pull request has changed — a verdict, a
// conflict, or the answering of one. A HINT: every author's map reads the record on its own beat.
func (a *Act) announceVerdict(project string) {
	a.Deps.Notify()
	a.Flow.WakeProject(project, topic.PRVerdict)
}

// run is the acting half of a run's flow: the quality gate a submit waits on is a queued run.
func (a *Act) run() *run.Act { return run.New(a.Core) }

// task is the acting half of a task's flow: a merge settles the task the branch was for.
