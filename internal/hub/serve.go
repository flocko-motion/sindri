// package: hub / serve
// type:    logic (the hub's control socket)
// job:     bring the control socket up — re-serve every agent's channel, reconcile whatever was
// in flight when the hub last died, seed each project's task cache — then serve the
// front-end surface on it until the listener closes.
// limits:  the boot and the listener. The routes themselves are the front-end's
// (-> hub/api/frontend), and the agents' channel is its own (-> hub/api/agents/channel).
package hub

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"

	"github.com/flo-at/sindri/internal/hub/api/frontend"
	"github.com/flo-at/sindri/internal/hub/api/serve"
)

// Handler builds the front-end's mux over this hub.
func (h *Hub) Handler() http.Handler { return frontend.Handler(h) }

// Serve binds the repo's unix socket and serves until the listener closes. A
// stale socket file from a previous run is removed first.
func (h *Hub) Serve() error {
	// Re-serve every rostered agent's socket so a restarted hub recovers all
	// agent channels (D11).
	if err := h.agentCh.ServeAgents(); err != nil {
		return err
	}
	// macOS: unix sockets can't cross the podman VM boundary, so also serve the
	// agent surface over a loopback TCP channel (token-authenticated).
	if runtime.GOOS == "darwin" {
		if err := h.agentCh.ServeTCP(); err != nil {
			return err
		}
	}
	// A planner holding a backlog task and a merge left in flight are both repaired by the maps that
	// own them now — the planner's own disowning state, and pr/merging's orphan exit — so nothing is
	// asked for here that is only reachable at boot.
	h.RunFlow().ReconcileRunningRuns(h.lifetime) // a run in flight when we last died → failed (outcome unknown)
	// Seed each known project's task cache so its board is populated from the start.
	// A per-project failure (typically no td store at that repo) is not fatal — the
	// hub still serves agents/PRs — but it must be loud, not silent.
	known, kerr := h.projects.Known()
	if kerr != nil {
		fmt.Fprintf(os.Stderr, "hub: WARNING — could not read the repo registry: %v\n", kerr)
	}
	for _, p := range known {
		if err := h.TaskFlow().SyncTasks(p.Tag); err != nil {
			fmt.Fprintf(os.Stderr, "hub: WARNING — could not load tasks for %s: %v\n", p.Path, err)
		}
	}
	path := h.SocketPath()
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	return http.Serve(ln, serve.LogRequests("hub", frontend.RequireProject(h.Handler())))
}
