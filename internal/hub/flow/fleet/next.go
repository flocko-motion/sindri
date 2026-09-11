// package: hub/flow/fleet / next
// type:    logic (the verb that asks for work)
// job:     answer an agent typing `next` — refresh the backlog, announce it, and report where the
// agent then stands.
// limits:  the fact and the announcement. Nothing here claims anything: which unit an agent takes,
// and whether it may take one at all, is its own map's (-> roles/worker's idle and assigning).
package fleet

import (
	"fmt"
	"io"

	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
)

// CmdNext asks for the next piece of work. It claims nothing: the backlog is re-synced because the
// task sources announce nothing of their own, the topic says so, and the agent's own map decides
// what that changes. A worker standing idle with a unit rated for it is moved into the hand-over,
// which speaks its own brief — so what this reports is simply where the agent has landed.
//
// A second claimer here is what this verb used to be, and the reason it is gone: it made the SAME
// decision as the map, from a copy of the rules, and the two disagreed about a retired agent, an
// armed clear and a pod that was not running.
func (e *Engine) CmdNext(c registry.Caller, _ []string, out io.Writer) (int, error) {
	_ = e.taskAct().SyncTasks(c.Project) // best-effort: the cached set answers if a source is unreachable
	e.WakeProject(c.Project, topic.TaskAvailable)
	e.Flow.Look(c.Project, c.Agent)
	preamble, err := e.roleAct().ServeMail(c.Project, c.Agent)
	if err != nil {
		return 1, err
	}
	words, err := e.stands(c.Project, c.Agent)
	if err != nil {
		return 1, err
	}
	fmt.Fprintln(out, preamble+words)
	return 0, nil
}
