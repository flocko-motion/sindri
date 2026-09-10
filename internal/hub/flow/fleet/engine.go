// package: hub/flow/fleet / engine
// type:    assembly (one running machine per subject, over the hub's handles)
// job:     hold the four machines — agents, tasks, pull requests, runs — and be the one thing every
// acting half re-decides through (-> core.Flows). It DECIDES nothing: the maps beside it do, and
// each subject's *_act.go does the writing.
// limits:  assembly and the looks that drive it. Every rule belongs to a map, every write to an
// acting half, and git/persistence to adapter/git and hub/store.
package fleet

import (
	"context"
	agentflow "github.com/flo-at/sindri/internal/hub/flow/agent"
	prflow "github.com/flo-at/sindri/internal/hub/flow/pr"
	taskflow "github.com/flo-at/sindri/internal/hub/flow/task"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"

	"github.com/flo-at/sindri/internal/adapter/tasks"
	"github.com/flo-at/sindri/internal/hub/core"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
	flowpr "github.com/flo-at/sindri/internal/hub/flow/pr"
	runflow "github.com/flo-at/sindri/internal/hub/flow/run"
	flowtask "github.com/flo-at/sindri/internal/hub/flow/task"
	"github.com/flo-at/sindri/internal/hub/world/situation"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// TaskSourceToolMissing reports a task source wanting a tool the repo calls for but that is not on
// PATH — an openspec/ dir with no openspec CLI. Project-agnostic sources only.
func (e *Engine) TaskSourceToolMissing(root string) bool {
	for _, src := range e.Sources {
		if src.ToolMissing(root) {
			return true
		}
	}
	return false
}

// Harness and Deps are the hub's two seams, declared in core and named here under the names every
// caller already uses. A subject package takes a *core.Core rather than reaching for these.
type (
	Harness = core.Harness
	Deps    = core.Deps
)

// Engine is the workflow orchestrator: it owns the store and drives the lifecycle
// steps, reaching the rest of the hub through Deps.
type Engine struct {
	// The handles every subject needs, held in one place so a subject package can be handed that one
	// thing when it leaves (-> hub/core). Embedded, so this reads as e.Store and travels as c.Store.
	*core.Core
	flow    machine.Machine[flow.World]     // an agent's declared flow (-> flowmachine.go)
	tasks   machine.Machine[flowtask.World] // a task's own (-> flowtask.go)
	prs     machine.Machine[flowpr.World]   // a merge intent's own (-> flowpr.go)
	runs    machine.Machine[runflow.World]  // a queued run's own (-> run.go)
	sources []tasks.Source                  // external task sources, wired in at New; owned.OwnedSource is always added per-project
}

// Handles are the hub's shared handles, for a subject package that acts over them. The Engine is on
// its way to being nothing but these plus what has not moved out yet.
func (e *Engine) Handles() *core.Core { return e.Core }

// prAct, roleAct and taskAct are the acting halves of the flows a still-unmoved verb reaches into.
// Built per call: they hold nothing of their own beyond the handles below them.
func (e *Engine) prAct() *prflow.Act      { return prflow.New(e.Core) }
func (e *Engine) roleAct() *agentflow.Act { return agentflow.New(e.Core) }
func (e *Engine) taskAct() *taskflow.Act  { return taskflow.New(e.Core) }
func (e *Engine) runAct() *runflow.Act    { return runflow.New(e.Core) }

// Gated installs the submit path's quality gates on the engine's handles, chainable alongside New.
// An engine with none runs no gate.
func (e *Engine) Gated(gates ...core.Gate) *Engine {
	e.Core.WithGates(gates...)
	return e
}

// New builds the engine over the hub's lifetime, store, Deps and the task sources the composition
// root wires in. The lifetime is a parameter so no path invents a root of its own.
func New(lifetime context.Context, st *store.Store, deps Deps, hn Harness, box *mail.Box, sources ...tasks.Source) *Engine {
	e := &Engine{
		Core: &core.Core{
			Store: st, Deps: deps, Harness: hn,
			Sit: situation.NewGatherer(st, hn), Lifetime: lifetime, Sources: sources, Mail: box,
			Pre: core.Preflight{Seen: map[string]string{}},
		},
	}
	m, err := e.newFlow(lifetime, 0)
	if err != nil {
		// The flows are declared in code, so this cannot depend on anything a user did: a map that
		// will not assemble is a build-time mistake reaching runtime, and it stops here.
		panic("workflow: the declared flow is inconsistent: " + err.Error())
	}
	e.flow = m
	tm, terr := e.newTaskFlow(lifetime, 0)
	if terr != nil {
		panic("workflow: the declared task flow is inconsistent: " + terr.Error())
	}
	e.tasks = tm
	pm, perr := e.newPRFlow(lifetime, 0)
	if perr != nil {
		panic("workflow: the declared pull-request flow is inconsistent: " + perr.Error())
	}
	e.prs = pm
	rm, rerr := e.runAct().NewRunFlow(lifetime, 0)
	if rerr != nil {
		panic("workflow: the declared run flow is inconsistent: " + rerr.Error())
	}
	e.runs = rm
	// Last: an act that changes the world tells the machines to look again, and they must EXIST
	// before anything can be handed a way to reach them.
	e.Core.Flow = e
	// The gate half of a queued run: the PR decides what a passing verdict unlocks.
	e.Core.Gate = prflow.New(e.Core)
	return e
}

// mockSpecTask is the placeholder todo id on a planner's openspec PR (there's no
// real backlog task behind it).
const mockSpecTask = "os-new"
