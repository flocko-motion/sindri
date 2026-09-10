// package: hub/flow/pr / handles_act
// type:    assembly (the acting half of a pull request's flow)
// job:     hold what acting on a merge intent needs — the hub's handles — so the map beside this
// stays a declaration and everything that WRITES is on one type.
// limits:  acting. What a merge intent's states mean is pr.go's, and this decides nothing.
package pr

import (
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/flow/run"
)

// Act performs what a pull request's flow decides.
type Act struct{ *core.Core }

// New builds the acting half over the hub's handles.
func New(c *core.Core) *Act { return &Act{Core: c} }

// run is the acting half of a run's flow: the quality gate a submit waits on is a queued run.
func (a *Act) run() *run.Act { return run.New(a.Core) }

// task is the acting half of a task's flow: a merge settles the task the branch was for.
