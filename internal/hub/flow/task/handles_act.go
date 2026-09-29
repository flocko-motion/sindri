// package: hub/flow/task / handles_act
// type:    assembly (the acting half of a task's flow)
// job:     hold what acting on a task needs — the hub's handles — so the map beside this stays a
// declaration and everything that WRITES is on one type.
// limits:  acting. What a task's states mean is task.go's, and it decides nothing.
package task

import (
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/flow/pr"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
)

// Act performs what a task's flow decides. Named for what it does, and separate from the map so the
// purity of the map is a property of the FILE rather than a discipline (-> internal/arch).
type Act struct{ *core.Core }

// New builds the acting half over the hub's handles.
func New(c *core.Core) *Act { return &Act{Core: c} }

// announceHolding says what an agent holds has changed, so its own map re-decides where that leaves
// it. A HINT, like every topic: the row is read again on the agent's own beat regardless.
func (a *Act) announceHolding(project, agent string) {
	a.Deps.Notify()
	a.Flow.Wake(project, agent, topic.HoldingChanged)
}

// pr is the acting half of a pull request's flow: ending a task settles what was open against it.
func (a *Act) pr() *pr.Act { return pr.New(a.Core) }
