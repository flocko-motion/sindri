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

	"github.com/flo-at/sindri/internal/hub/store"
)

// Deps is what the agent module needs back from the hub: wake the board, and resolve
// an agent's container name (the hub owns the repo-scoped naming scheme).
type Deps interface {
	Notify()
	ContainerName(project, name string) string
}

// Service is the agent-management module: identity (naming), auth (tokens), memory
// config, launch-output capture, and message injection into a running agent — the
// hub-side mechanics of managing an agent, over the central store.
type Service struct {
	store *store.Store
	deps  Deps

	launchMu sync.Mutex             // guards launch
	launch   map[string]*safeBuffer // per-agent launch-output buffers (see launchbuf.go)
}

// New builds the agent module over the hub's store and its Deps.
func New(st *store.Store, deps Deps) *Service {
	return &Service{store: st, deps: deps, launch: map[string]*safeBuffer{}}
}
