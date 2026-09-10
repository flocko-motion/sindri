// package: hub/flow/task / handles_act
// type:    assembly (the acting half of a task's flow)
// job:     hold what acting on a task needs — the hub's handles — so the map beside this stays a
// declaration and everything that WRITES is on one type.
// limits:  acting. What a task's states mean is task.go's, and it decides nothing.
package task

import (
	"github.com/flo-at/sindri/internal/hub/core"
	agentflow "github.com/flo-at/sindri/internal/hub/flow/agent"
	"github.com/flo-at/sindri/internal/hub/flow/pr"
)

// Act performs what a task's flow decides. Named for what it does, and separate from the map so the
// purity of the map is a property of the FILE rather than a discipline (-> internal/arch).
type Act struct{ *core.Core }

// New builds the acting half over the hub's handles.
func New(c *core.Core) *Act { return &Act{Core: c} }

// pr is the acting half of a pull request's flow: ending a task settles what was open against it.
func (a *Act) pr() *pr.Act { return pr.New(a.Core) }

// roles is the acting half of an agent's own flow: handing work over prepares the session it
// lands in, and that preparation is the agent's business rather than the task's.
func (a *Act) roles() *agentflow.Act { return agentflow.New(a.Core) }
