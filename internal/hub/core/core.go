// package: hub/core / core
// type:    assembly (the handles every subject needs)
// job:     hold what the hub gives its subjects — the store, the box its agents live in, the hub's
// own facts, and where each agent stands — so a package that owns one subject can be handed one
// thing rather than reaching into an object that owns them all.
// limits:  holding. Nothing here decides anything, and no subject's rules live here.
package core

import (
	"context"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/adapter/tasks"
	"github.com/flo-at/sindri/internal/api"

	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/world/observe"
	"github.com/flo-at/sindri/internal/hub/world/owned"
	"github.com/flo-at/sindri/internal/hub/world/situation"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

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
	Say(project, name, text string, d mail.Delivery) error
	// Clear and SetModel reset or steer the session, each blocking until it takes effect or times
	// out (-> agent-runtime's blocking-command contract). There is no compacting: a session worth
	// resetting is cleared, and one that is not is left alone.
	Clear(ctx context.Context, project, name string) error
	SetModel(ctx context.Context, project, name, model string) error
	// Interrupt aborts whatever the session is doing (ESC), so a notice lands on an idle prompt
	// rather than queuing behind work.
	Interrupt(project, name string) error
	// Start brings a stopped agent back up, its session resuming.
	Start(project, name string) error
	// Container names an agent's box.
	Container(project, name string) string
	// ModelMatches is the BACKEND's own knowledge of its models: whether two ids name one. Here
	// rather than on Deps because only the thing running the session knows it — the hub's POLICY
	// about models, which model a difficulty tier deserves, sits on Deps instead (-> tasks.md 3.2).
	ModelMatches(want, detected string) bool
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

// Flows is the re-decision seam: a subject that CHANGED the world tells the machines whose states
// read it to look again. Named for what it does rather than for the machine behind it — an act
// knows it moved something, never which map that unsettles.
type Flows interface {
	// Look settles one agent's own machine, synchronously; Wake nudges it on a topic.
	Look(project, agent string)
	Wake(project, agent string, topic machine.Topic)
	// LookPRs and LookTask settle the machines of a project's pull requests and of one task.
	LookPRs(project string)
	LookTask(project, id string)
	// LookTasks settles every task machine in a project.
	LookTasks(project string)
	// WakeProject and WakeAll nudge every subscriber to a topic, in a project or across the fleet.
	WakeProject(project string, topic machine.Topic)
	WakeAll(topic machine.Topic)
	// WakeRuns nudges the one fleet-wide run queue.
	WakeRuns(topic machine.Topic)
}

// Gates runs the gate half of the queue. A run whose Kind names a gate is executed by the subject
// that will ACT on its verdict — the queue holds the slot, the PR decides what a pass unlocks.
type Gates interface {
	ExecuteGateRun(ctx context.Context, project string, r api.Run) error
	// CompleteGate lands what a finished gate unlocks — a PR, feedback to fix, a lint reply.
	CompleteGate(project string, r api.Run, status, output string) error
}

// Core is what a subject package is handed. The store is imported rather than wrapped: it is a data
// adapter with one implementation and no plausible second, so an interface over it would hide
// nothing and vary in nothing (-> ARCHITECTURE.md, on ports).
type Core struct {
	Store   *store.Store
	Harness Harness
	Deps    Deps
	Sit     *situation.Gatherer
	// Flow re-decides what an act has changed. Set by the composition root once the machines exist,
	// so it is nil for the moment between building the handles and wiring them.
	Flow Flows
	// Gate executes a queued gate. Set by the composition root beside Flow.
	Gate Gates
	// Mail is the fleet's mailbox, for a subject that has something to say. Set beside Flow, and the
	// one door out: nothing here writes a message itself (-> hub/messaging/mail).
	Mail *mail.Box
	// Lifetime is the hub's own, for FLEET-SIDE work with no caller context to inherit — a push
	// through a port whose signature carries none. Anything reached from an agent's own request
	// takes that request's context instead.
	Lifetime context.Context

	// Sources is where a project's tasks come from, ordered — sindri's own first. Held here because
	// every subject that CLOSES or SYNCS a task walks the same list.
	Sources []tasks.Source

	// Kills is the set of runs killed mid-execution, so the blocked exec call can tell a kill from
	// an ordinary failure. Held here because it outlives any one acting half.
	Kills Kills

	// Prodded is the idle spell each agent was last prodded for, so a stall is named ONCE however
	// many callers notice it. Held here because it outlives any one acting half.
	Prodded Spells

	// Pre serialises the reference-move PR checks, so one sweep does not queue the same check twice.
	Pre Preflight

	// gates are the submit path's quality validators, installed by the composition root (-> WithGates).
	gates []Gate

	// refWarn remembers which repo roots have already been warned about an unconfigured reference
	// (-> BaseBranch), so a call on every submit does not spam the log with the same finding.
	refWarn refFallbackWarn
}

// PlannerBranch is a planner's standing branch, where it drafts openspec and ships it. Exported
// because the hub's launch path lays it down when it starts a planner.
func PlannerBranch(name string) string { return "plan-" + name }

// LogCap bounds a commit listing — enough to see what moved, not a project history.
const LogCap = 40

// TaskSources is the ordered set of task backends a project syncs from and notifies on merge,
// self-filtered by id scheme; sindri's own always leads, the rest are whatever the root wired in.
func (c *Core) TaskSources(project string) []tasks.Source {
	return append([]tasks.Source{owned.Over(c.Store.For(project))}, c.Sources...)
}

// RestPhase is an agent's resting phase: "planning" for a planner, "collab" for a coauthor
// (neither holds a backlog task, so "idle" would mislead), "idle" for everyone else.
func RestPhase(role string) string {
	switch role {
	case "planner":
		return "planning"
	case "coauthor":
		return "collab"
	default:
		return "idle"
	}
}

// Kills tracks run ids killed mid-execution, so the blocked ExecContext call can tell a
// kill from an ordinary failure without a second write racing ExecuteRun's own finish.
type Kills struct {
	mu  sync.Mutex
	ids map[string]bool
}

// Request marks id as killed on purpose.
func (s *Kills) Request(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ids == nil {
		s.ids = map[string]bool{}
	}
	s.ids[id] = true
}

// consume reports whether id was requested, clearing it either way so the set never grows past
// however many runs are cancelled and not yet finished (at most one, at concurrency one).
func (s *Kills) Consume(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	ok := s.ids[id]
	delete(s.ids, id)
	return ok
}

// Preflight serialises the DECIDING and remembers what has already been answered — one PR per sweep,
// so a moving base cannot queue a check per open PR at once. What keeps two checks from building at
// the same time is the run queue they now go through, not this mutex (-> executePrecheckRun).
type Preflight struct {
	Mu   sync.Mutex
	Seen map[string]string // PR id -> the base and tips already checked (-> prCheckKey)
}

// MockSpecTask is the placeholder todo id on a planner's openspec PR: there is no backlog task
// behind it, and every planner's spec PR names the same one.
const MockSpecTask = "os-new"

// ClearArmedFor reports whether a human has armed a context clear. Read off the situation rather than
// the roster row, so the fact and every rule built on it come from one place.
func (c *Core) ClearArmedFor(project, name string) bool {
	s, err := c.Sit.Of(project, name)
	return err == nil && s.ClearArmed
}

// Retired reports a human-parked agent — hands off every automatic behaviour, written once so a
// feature added later asks this instead of keeping its own copy (-> retire.go).
func (c *Core) Retired(project, name string) bool {
	s, err := c.Sit.Of(project, name)
	return err == nil && s.Retired
}

// Spells remembers, per agent, the idle spell it was last prodded for. Keyed on the SPELL rather
// than a timestamp so one stall is named once — and a NEW stall, which starts a new spell, is
// named again.
type Spells struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

// First reports whether this is a spell nobody has prodded for yet, and claims it if so.
func (s *Spells) First(key string, spell time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[key].Equal(spell) {
		return false
	}
	if s.seen == nil {
		s.seen = map[string]time.Time{}
	}
	s.seen[key] = spell
	return true
}

// Moving forgets an agent, so the next stall is a new one.
func (s *Spells) Moving(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.seen, key)
}
