// package: hub/workflow / engine
// type:    logic (the workflow state machine — orchestration)
// job:     the explicit orchestrator of sindri's PR/task lifecycle: claim → work →
// submit → review → approve → merge, plus task create/edit/close and the
// worker directive loop. It sequences the steps and triggers the actions in
// the other modules (repo, store, agent messaging) via a narrow Deps seam.
// limits:  git/PR mechanics live in hub/repo; persistence in hub/store; the hub owns
// the Deps implementation, pods, and transport. No git or tmux here.
package workflow

import (
	"github.com/flo-at/sindri/internal/adapter/gate"
	"github.com/flo-at/sindri/internal/adapter/tasks"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/store"
)

// taskSources is the ordered set of task backends the workflow syncs from and notifies on merge.
// Each self-filters by id scheme, so the workflow treats them uniformly and never branches on which
// concrete source is underneath a task. ownedSource always leads, project-scoped, because the source
// sindri owns reads the hub's own store rather than a tool in the repo; the rest are whatever the
// composition root wired in at New (github, openspec, ...) — the engine learns nothing about them.
func (e *Engine) taskSources(project string) []tasks.Source {
	return append([]tasks.Source{ownedSource{e.store.For(project)}}, e.sources...)
}

// TaskSourceToolMissing reports whether any task source wants a tool the repo's content calls for
// but that isn't on PATH — e.g. an openspec/ dir with no openspec CLI installed. Project-agnostic
// sources only (ownedSource never has one), so it does not need a project to scope by.
func (e *Engine) TaskSourceToolMissing(root string) bool {
	for _, src := range e.sources {
		if src.ToolMissing(root) {
			return true
		}
	}
	return false
}

// Deps is the seam the workflow needs back into the hub — everything the
// orchestration touches that isn't the store (held directly) or another workflow
// step. The hub supplies the implementation; this keeps the workflow package free of
// the hub's transport, pods, and tmux.
type Deps interface {
	// ProjectRoot resolves a project (repoTag) to its on-disk repo root.
	ProjectRoot(project string) string
	// ProjectConfig returns a project's resolved .sindri config.
	ProjectConfig(project string) (config.Config, error)
	// ArchitectureDoc returns a project's repo-relative architecture doc path.
	ArchitectureDoc(project string) string
	// Container returns an agent's container name.
	Container(project, name string) string
	// Notify wakes the board (an SSE change notification).
	Notify()
	// InjectWhenReady delivers a message into an agent's session once it's ready.
	InjectWhenReady(project, name, text string) error
	// Interrupt aborts an agent's current operation (sends ESC to its session), so a
	// scrapped-task notice lands on an idle prompt rather than queuing behind work.
	Interrupt(project, name string) error
	// AgentAlive reports whether an agent's pod is currently running.
	AgentAlive(project, name string) bool
	// AgentIdle reports an agent sitting at an empty prompt, from the watchdog's last observation.
	// What it answers is whether a message sent NOW would be acted on: text typed into a running
	// turn lands in the input box and dies there when the turn ends.
	AgentIdle(project, name string) bool
	// SessionAlive reports whether an agent's tmux session is live.
	SessionAlive(project, name string) bool
	// TaskComments returns a task's comments for display.
	TaskComments(project, id string) []store.Comment
	// AddTaskComment posts on a task's thread as author — the write half of TaskComments, for a
	// workflow step whose record belongs where the user reads it rather than in an agent's log.
	AddTaskComment(project, id, author, body string) error
	// Subscribe returns a change-notification channel and an unsubscribe func — how
	// the directive loop waits for work.
	Subscribe() (chan struct{}, func())
	// KnownProjects returns the registered repos (for fleet-wide PR listing).
	KnownProjects() []store.Project
	// BrokkrBin locates the brokkr toolbelt binary (the lint gate shells out to it).
	BrokkrBin() (string, error)
	// ContextUsage reports an agent's current session context size and the window it fills, both
	// read off its transcript. ok=false when nothing has been recorded yet.
	ContextUsage(project, name string) (tokens, window int, ok bool)
}

// clearArmed reports whether a human has armed a context clear for this agent. It is handed no new
// leaf work while that stands: the clear fires at the boundary it is already at, and a task claimed
// in between would be cut in half by it (-> agent.Service.SetClearArmed).
func (e *Engine) clearArmed(project, name string) bool {
	a, ok, err := e.store.For(project).GetAgent(name)
	return err == nil && ok && a.ClearArmed
}

// Engine is the workflow orchestrator: it owns the store and drives the lifecycle
// steps, reaching the rest of the hub through Deps.
type Engine struct {
	store      *store.Store
	deps       Deps
	sources    []tasks.Source // external task sources, wired in at New; ownedSource is always added per-project
	gates      []gate.Gate    // submit-path quality gates, wired in via WithGates; openspec today
	pre        preflight      // serialises the reference-move PR checks (-> prcheck.go)
	runCancels runCancelSet   // run ids killed mid-execution (-> execrun.go)
}

// New builds the workflow engine over the hub's store, its Deps implementation, and the external
// task sources the composition root wires in (github, openspec, ...) — the engine never names them.
func New(st *store.Store, deps Deps, sources ...tasks.Source) *Engine {
	return &Engine{store: st, deps: deps, sources: sources, pre: preflight{seen: map[string]string{}}}
}

// WithGates installs the submit path's quality gates — openspec validation today, the built-in
// lint gate once its own adapter task lands (the same seam, a second Gate). Chainable, so the
// composition root wires it in alongside New in one line. An engine with none runs no gate.
func (e *Engine) WithGates(gates ...gate.Gate) *Engine {
	e.gates = gates
	return e
}

// qualityGate runs every installed gate against wt, stopping at the first failure. A gate whose
// domain doesn't apply to this repo (e.g. openspec with no openspec/ dir) reports ok=true itself.
func (e *Engine) qualityGate(wt string) (ok bool, output string) {
	for _, g := range e.gates {
		if ok, out := g.Validate(wt); !ok {
			return false, out
		}
	}
	return true, ""
}

// mockSpecTask is the placeholder todo id on a planner's openspec PR (there's no
// real backlog task behind it).
const mockSpecTask = "os-new"

// PlannerBranch is a planner's standing branch — it drafts openspec here and ships it
// via `openspec submit` (it never grabs a backlog task). Exported because the hub's
// launch path lays this branch down when it starts a planner.
func PlannerBranch(name string) string { return "plan-" + name }

// restPhase is an agent's resting (not-busy) phase: a planner rests in "planning" and
// a coauthor in "collab" (neither holds a backlog task, so "idle" would mislead —
// they're standing with the user, not unoccupied); everyone else "idle".
func restPhase(role string) string {
	switch role {
	case "planner":
		return "planning"
	case "coauthor":
		return "collab"
	default:
		return "idle"
	}
}

// verifyCmd is the project's declared gate command, or "" when it declares none (or the config
// cannot be read — an unreadable config must not silently disable a project's own gate, so the
// built-ins still run and the config error surfaces where configs are loaded).
func (e *Engine) verifyCmd(project string) string {
	cfg, err := e.deps.ProjectConfig(project)
	if err != nil {
		return ""
	}
	return cfg.Verify
}
