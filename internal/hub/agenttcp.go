// package: hub / agenttcp
// type:    logic (macOS agent channel over TCP)
// job:     on macOS, where a bind-mounted unix socket can't cross the VM boundary,
//
//	serve the agent surface over a TCP listener instead, authenticated by a
//	per-agent bearer token (token.go). Where it binds and how a pod reaches
//	it come from the wired runtime's AgentChannel (podman vs apple container).
//
// limits:  Linux keeps per-agent unix sockets (agentserver.go); this runs only on
//
//	darwin. Same handler surface, token-resolved caller.
package hub

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/flo-at/sindri/internal/container"
)

// tcpPortMetaKey names the persisted TCP port in the store's meta table.
const tcpPortMetaKey = "agent_tcp_port"

// serveAgentTCP starts the loopback TCP agent channel and records its port so
// Launch can hand it to pods. It reuses the previously-bound port when free, so a
// hub restart keeps the same address and already-running pods stay connected (D11);
// only if that port is taken does it fall back to an ephemeral one.
func (h *Hub) serveAgentTCP() error {
	// The wired runtime decides where to bind and how a pod addresses the host —
	// podman uses loopback + host.containers.internal, apple container the bridge
	// gateway IP. A failure to determine this is fatal (the channel is unusable).
	ch, err := container.AgentChannel()
	if err != nil {
		return fmt.Errorf("determine agent network channel: %w", err)
	}
	h.agentDialHost = ch.DialHost
	port := 0
	if v, ok, _ := h.store.GetMeta(tcpPortMetaKey); ok {
		port, _ = strconv.Atoi(v)
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(ch.BindAddr, strconv.Itoa(port)))
	if err != nil && port != 0 { // saved port unavailable — take any free one
		ln, err = net.Listen("tcp", net.JoinHostPort(ch.BindAddr, "0"))
	}
	if err != nil {
		return fmt.Errorf("serve agent tcp on %s: %w", ch.BindAddr, err)
	}
	h.agentTCPLn = ln
	h.agentTCPPort = ln.Addr().(*net.TCPAddr).Port
	if err := h.store.SetMeta(tcpPortMetaKey, strconv.Itoa(h.agentTCPPort)); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "hub: agent TCP channel on %s:%d, pods dial %s (macOS; token-authenticated)\n",
		ch.BindAddr, h.agentTCPPort, h.agentDialHost)
	go http.Serve(ln, logRequests("agent-tcp", h.agentTCPHandler()))
	return nil
}

// agentTCPHandler authenticates the bearer token, resolves it to an agent, and
// dispatches to that agent's usual handler — so the token stands in for the unix
// socket's "the endpoint is the identity".
func (h *Hub) agentTCPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project, name, ok, err := h.agentForToken(r.Header.Get("X-Sindri-Token"))
		if err != nil { // a store failure is 500, not "bad token" — and it's logged, not swallowed
			log.Printf("agent-tcp: token lookup failed: %v", err)
			http.Error(w, `{"error":"token lookup failed"}`, http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, `{"error":"unauthorized: bad or missing agent token"}`, http.StatusUnauthorized)
			return
		}
		h.agentHandler(project, name).ServeHTTP(w, r)
	})
}
