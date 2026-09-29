// package: hub/harness / service
// type:    logic (the agent as an actor — identity, auth, resource config)
// job:     the hub-side facts about an agent that are NOT the coding tool itself
// (that's the adapter/agent port): allocate its machine-unique name, derive
// and resolve its bearer token, and read/set its pod memory limit. Backed by
// the central store; wakes the board on a change.
// limits:  no transport or launch here (-> the hub wires listeners and pods); the
// coding agent's own behaviour is the adapter's (-> adapter/agent).
package harness

import (
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"sync"

	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/api/agents/channel"
	"github.com/flo-at/sindri/internal/hub/world/observe"
	"github.com/flo-at/sindri/internal/hub/world/situation"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// Deps is what the agent-management module needs back from the hub: the board, an agent's container
// name and its repo's facts, a task refresh, and the post-launch nudge. The lifecycle mechanics live
// here; these are the few hub facilities they cannot own.
type Deps interface {
	Notify()
	ContainerName(project, name string) string
	ProjectRoot(project string) string
	ProjectConfig(project string) (config.Config, error)
	ArchitectureDoc(project string) string
	RefreshTask(project, id string) error
	// SetTaskStatus moves a task to want without the caller knowing WHERE that status lives — the
	// owned table for a task sindri owns, the source for a mirrored one. Asked rather than branched
	// on: every site that grew its own "if OwnsTask" is a site that will one day forget.
	SetTaskStatus(project, id, want string) error
	Rehydrate(project, name string)
	// Kickoff is what a session coming up fresh is told, resolved from the agent's role by the hub
	// (-> fleet.Engine.Kickoff) — the same division as Rehydrate: here the WHEN, there the WHAT.
	Kickoff(project, name string) string
	// Deliver is the hub's ordinary delivery path, the one that reports its own failures — how a
	// command that reset a session sends what follows.
	Deliver(project, name, text string, d mail.Delivery) error
	// ForgetFill drops the hub's standing sample of an agent's context fill. The board reports THAT
	// sample, not this package's memo, so invalidating one without the other leaves the stale figure.
	ForgetFill(project, name string)
	// Observation is the hub's standing look at an agent, memoised (-> situation.Situation). Named for
	// the value, not the verb: this Service's own Observe PROBES, and the two must not read alike.
	Observation(project, name string) observe.Observation
	// AgentUp is the watchdog's last liveness reading — what the hub's idle/clear ticks read instead
	// of AgentAlive, sparing a probe per roster member per tick.
	AgentUp(project, name string) bool
	// AgentClients is the watchdog's last dialed-in count, for the same reason.
	AgentClients(project, name string) int
}

// Service is the agent-management module: identity, auth, memory config, launch-output capture,
// injection, runtime inspection and the pod lifecycle. Triggers come from outside; this does the work.
type Service struct {
	store   *store.Store
	deps    Deps
	agentCh *channel.Server // the agent command channel (served per launch, closed on delete)

	launchMu sync.Mutex             // guards launch
	launch   map[string]*safeBuffer // per-agent launch-output buffers (see launchbuf.go)

	askMu sync.Mutex           // guards asks
	asks  map[lcKey]*LaunchAsk // a human's launch request awaiting the machine (see launchask.go)

	lcMu      sync.Mutex                // guards lifecycle
	lifecycle map[lcKey]lifecycleIntent // transient launch/stop intent: "launching"|"stopping"|failed

	sit *situation.Gatherer // where an agent stands, and what may happen to it (-> hub/situation)

	runtimeMemo runtimeMemo // Observe's TTL cache (runtime.go)
	contextMemo contextMemo // ContextUsage's TTL cache (runtime.go)
	paneMemo    paneMemo    // AgentPane's TTL cache (runtime.go)
	reach       reachMemo   // consecutive pushes an agent's pane never showed (inject.go)
}

// observerFunc adapts Deps' accessor to the gatherer's port. Called through at USE time rather than
// bound at construction, so a fixture built with no Deps still constructs.
type observerFunc func(project, name string) observe.Observation

func (f observerFunc) Observe(project, name string) observe.Observation { return f(project, name) }

// New builds the agent module over the hub's store, its Deps, and the agent channel
// (the lifecycle serves/closes an agent's socket through it).
func New(st *store.Store, deps Deps, agentCh *channel.Server) *Service {
	return &Service{
		store: st, deps: deps, agentCh: agentCh,
		// No idle rule: the harness reads a situation for what a session RUNS — its model, its tier —
		// and never asks whether an agent has stopped doing anything. That question belongs to the
		// sweep, which is handed the flows' own declaration (-> agent.NudgesWhenIdle).
		sit: situation.NewGatherer(st, observerFunc(func(project, name string) observe.Observation {
			return deps.Observation(project, name)
		}), nil),
		launch:    map[string]*safeBuffer{},
		asks:      map[lcKey]*LaunchAsk{},
		lifecycle: map[lcKey]lifecycleIntent{},
	}
}
