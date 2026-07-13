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

// Service is the agent-actor module: naming, token auth, memory config, and per-agent
// launch-output capture over the hub's central store. Constructed once and wired into
// the hub.
type Service struct {
	store  *store.Store
	notify func()

	launchMu sync.Mutex             // guards launch
	launch   map[string]*safeBuffer // per-agent launch-output buffers (see launchbuf.go)
}

// New builds the agent-actor service over the hub's store. notify wakes the board
// after a state change; a nil notify is treated as a no-op.
func New(st *store.Store, notify func()) *Service {
	if notify == nil {
		notify = func() {}
	}
	return &Service{store: st, notify: notify, launch: map[string]*safeBuffer{}}
}
