// package: hub/agent / service
// type:    logic (the agent as an actor — identity, auth, resource config)
// job:     the hub-side facts about an agent that are NOT the coding tool itself
//          (that's the adapter/agent port): allocate its machine-unique name, derive
//          and resolve its bearer token, and read/set its pod memory limit. Backed by
//          the central store; wakes the board on a change.
// limits:  no transport or launch here (-> the hub wires listeners and pods); the
//          coding agent's own behaviour is the adapter's (-> adapter/agent).
package agent

import (
	"sync"

	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/hub/agentchan"
	"github.com/flo-at/sindri/internal/hub/store"
)

// Deps is what the agent-management module needs back from the hub: wake the board,
// resolve an agent's (repo-scoped) container name and repo root/config/architecture,
// refresh a task's cache (workflow), and run the post-launch rehydrate nudge (the hub
// decides WHAT to inject). The lifecycle mechanics live here; these are the few hub
// facilities they can't own.
type Deps interface {
	Notify()
	ContainerName(project, name string) string
	ProjectRoot(project string) string
	ProjectConfig(project string) (config.Config, error)
	ArchitectureDoc(project string) string
	RefreshTask(project, id string) error
	Rehydrate(project, name string)
}

// Service is the agent-management module: identity (naming), auth (tokens), memory
// config, launch-output capture, message injection, runtime inspection, and the pod
// lifecycle (launch/stop/delete/rebuild) — the hub-side mechanics of managing an
// agent. Triggers come from outside (server/CLI/workflow); this owns the mechanics.
type Service struct {
	store   *store.Store
	deps    Deps
	agentCh *agentchan.Server // the agent command channel (served per launch, closed on delete)

	launchMu sync.Mutex             // guards launch
	launch   map[string]*safeBuffer // per-agent launch-output buffers (see launchbuf.go)

	lcMu      sync.Mutex        // guards lifecycle
	lifecycle map[lcKey]string  // transient launch/stop intent: "launching"|"stopping"
}

// New builds the agent module over the hub's store, its Deps, and the agent channel
// (the lifecycle serves/closes an agent's socket through it).
func New(st *store.Store, deps Deps, agentCh *agentchan.Server) *Service {
	return &Service{
		store: st, deps: deps, agentCh: agentCh,
		launch:    map[string]*safeBuffer{},
		lifecycle: map[lcKey]string{},
	}
}
