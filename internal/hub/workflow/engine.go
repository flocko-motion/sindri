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
	"context"

	"github.com/flo-at/sindri/internal/adapter/gate"
	"github.com/flo-at/sindri/internal/adapter/tasks"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/observe"
	"github.com/flo-at/sindri/internal/hub/situation"
	"github.com/flo-at/sindri/internal/hub/store"
)

// taskSources is the ordered set of task backends the workflow syncs from and notifies on merge,
// self-filtered by id scheme; ownedSource always leads, the rest are whatever New wired in.
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

// Harness is the agent's BOX and nothing else: looking at it, saying something to it, resetting or
// steering its session, starting it. It names no task, PR, review or verdict — a method that named
// one would be a rule the box has no business knowing, and a build check holds that line
// (-> internal/arch).
type Harness interface {
	// Observe is the standing look the observer already took, free. Probe takes a FRESH one, for a
	// caller that needs the answer as of now rather than as of the last sweep — the distinction the
	// old AgentAlive/AgentUp pair carried, kept because it is real.
	Observe(project, name string) observe.Observation
	Probe(project, name string) observe.Observation
	// Say puts a message to the agent the way d asks: mail keeps it until read, a push types it in
	// now (-> delivery.go). It carries out what it is given and reports what happened.
	Say(project, name, text string, d Delivery) error
	// Clear, Compact and SetModel reset or steer the session, each blocking until it takes effect or
	// times out (-> agent-runtime's blocking-command contract).
	Clear(ctx context.Context, project, name string) error
	Compact(ctx context.Context, project, name string) error
	SetModel(ctx context.Context, project, name, model string) error
	// Interrupt aborts whatever the session is doing (ESC), so a notice lands on an idle prompt
	// rather than queuing behind work.
	Interrupt(project, name string) error
	// Start brings a stopped agent back up, its session resuming.
	Start(project, name string) error
	// Container names an agent's box.
	Container(project, name string) string
	// ModelMatches and CompactionThreshold are the BACKEND's own knowledge of its models: whether two
	// ids name one model, and what fill is worth compacting for a window. Here rather than on Deps
	// because only the thing running the session knows either — the hub's POLICY about models, which
	// model a difficulty tier deserves, sits on Deps instead (-> tasks.md 3.2).
	ModelMatches(want, detected string) bool
	CompactionThreshold(window int) int
}

// Deps is the seam back into the rest of the hub: the project's facts, the board, the task thread.
// Everything here names something the ORCHESTRATOR owns; anything naming a keystroke is the
// harness's (above), and a method that names both is the next thing to pull apart.
type Deps interface {
	// ProjectRoot resolves a project (repoTag) to its on-disk repo root.
	ProjectRoot(project string) string
	// ProjectConfig returns a project's resolved .sindri config.
	ProjectConfig(project string) (config.Config, error)
	// ArchitectureDoc returns a project's repo-relative architecture doc path.
	ArchitectureDoc(project string) string
	// Notify wakes the board (an SSE change notification).
	Notify()
	// TaskComments returns a task's comments for display.
	TaskComments(project, id string) []store.Comment
	// AddTaskComment posts on a task's thread as author — TaskComments' write half.
	AddTaskComment(project, id, author, body string) error
	// Escalate stops an agent on a decision only the user can make, recording the question where a
	// later reader looks. The hub's own verb, so a hub-raised escalation is the agent's in every way.
	Escalate(project, name, question string) (task string, err error)
	// KnownProjects returns the registered repos (for fleet-wide PR listing).
	KnownProjects() []store.Project
	// ModelForTier resolves a difficulty tier to its model, ok=false if unrecognised. POLICY, unlike
	// the two model questions on Harness: which model a tier deserves is the hub's to decide.
	ModelForTier(tier string) (model string, ok bool)
}

// clearArmed reports whether a human has armed a context clear. Read off the situation rather than
// the roster row, so the fact and every rule built on it come from one place.
func (e *Engine) clearArmed(project, name string) bool {
	s, err := e.sit.Of(project, name)
	return err == nil && s.ClearArmed
}

// fireClearIfArmed fires an armed clear right now and wakes the agent once it has landed. A clear is
// a direct request, not tied to one assignment. It reports whether it fired, because the reply to
// THIS call would go into the session the clear has just discarded: a caller that fired answers
// DirPreparing and serves nothing it expects to be read.
func (e *Engine) fireClearIfArmed(ctx context.Context, project, name string) (fired bool, err error) {
	if !e.clearArmed(project, name) {
		return false, nil
	}
	if err := e.hn.Clear(ctx, project, name); err != nil {
		return false, err
	}
	return true, e.hn.Say(project, name, MsgKickoff, PushOnly)
}

// Engine is the workflow orchestrator: it owns the store and drives the lifecycle
// steps, reaching the rest of the hub through Deps.
type Engine struct {
	store      *store.Store
	deps       Deps
	hn         Harness
	sit        *situation.Gatherer // where an agent stands, and what may happen to it (-> hub/situation)
	sources    []tasks.Source      // external task sources, wired in at New; ownedSource is always added per-project
	gates      []gate.Gate         // submit-path quality gates, wired in via WithGates; openspec today
	pre        preflight           // serialises the reference-move PR checks (-> prcheck.go)
	runCancels runCancelSet        // run ids killed mid-execution (-> execrun.go)
	refWarn    refFallbackWarn     // which repo roots have already been warned about an unconfigured reference (-> pr.go)
}

// New builds the workflow engine over the hub's store, its Deps, and the external task sources the
// composition root wires in — the engine never names them.
func New(st *store.Store, deps Deps, hn Harness, sources ...tasks.Source) *Engine {
	return &Engine{store: st, deps: deps, hn: hn, sit: situation.NewGatherer(st, hn), sources: sources,
		pre: preflight{seen: map[string]string{}}, refWarn: refFallbackWarn{seen: map[string]bool{}}}
}

// WithGates installs the submit path's quality gates, chainable alongside New. An engine with
// none runs no gate.
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

// restPhase is an agent's resting phase: "planning" for a planner, "collab" for a coauthor
// (neither holds a backlog task, so "idle" would mislead), "idle" for everyone else.
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

// VerifyCmd is the project's declared gate command, "" when it declares none or the config cannot be
// read. Nothing stands in for it: an undeclared gate refuses every submit (-> repo.Gate).
func (e *Engine) VerifyCmd(project string) string {
	cfg, err := e.deps.ProjectConfig(project)
	if err != nil {
		return ""
	}
	return cfg.Verify
}
