// package: hub/workflow / engine
// type:    logic (the workflow state machine — orchestration)
// job:     the explicit orchestrator of sindri's PR/task lifecycle: claim → work →
//          submit → review → approve → merge, plus task create/edit/close and the
//          worker directive loop. It sequences the steps and triggers the actions in
//          the other modules (repo, store, agent messaging) via a narrow Deps seam.
// limits:  git/PR mechanics live in hub/repo; persistence in hub/store; the hub owns
//          the Deps implementation, pods, and transport. No git or tmux here.
package workflow

import (
	"github.com/flo-at/sindri/internal/adapter/tasks"
	"github.com/flo-at/sindri/internal/adapter/tasks/github"
	"github.com/flo-at/sindri/internal/adapter/tasks/spec"
	"github.com/flo-at/sindri/internal/adapter/tasks/td"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/store"
)

// taskSources is the ordered set of task backends the workflow syncs from and
// notifies on merge. Each self-filters by id scheme, so the workflow treats them
// uniformly and never branches on which concrete source is underneath a task.
func taskSources() []tasks.Source {
	return []tasks.Source{td.Source{}, spec.Source{}, github.Source{}}
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
	// AgentAlive reports whether an agent's pod is currently running.
	AgentAlive(project, name string) bool
	// SessionAlive reports whether an agent's tmux session is live.
	SessionAlive(project, name string) bool
	// TaskComments returns a task's comments for display.
	TaskComments(project, id string) []store.Comment
	// Subscribe returns a change-notification channel and an unsubscribe func — how
	// the directive loop waits for work.
	Subscribe() (chan struct{}, func())
	// KnownProjects returns the registered repos (for fleet-wide PR listing).
	KnownProjects() []store.Project
	// BrokkrBin locates the brokkr toolbelt binary (the lint gate shells out to it).
	BrokkrBin() (string, error)
}

// Engine is the workflow orchestrator: it owns the store and drives the lifecycle
// steps, reaching the rest of the hub through Deps.
type Engine struct {
	store *store.Store
	deps  Deps
}

// New builds the workflow engine over the hub's store and its Deps implementation.
func New(st *store.Store, deps Deps) *Engine { return &Engine{store: st, deps: deps} }

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
