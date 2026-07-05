// package: hub / agenttcp
// type:    logic (macOS agent channel over TCP)
// job:     on macOS, where a bind-mounted unix socket can't be connected to across
//          the podman VM boundary, serve the agent surface over a loopback TCP
//          listener instead. Identity is a per-agent bearer token (token.go), not
//          the socket path. Bound to 127.0.0.1; pods reach it via
//          host.containers.internal.
// limits:  Linux keeps per-agent unix sockets (agentserver.go); this runs only on
//          darwin. Same handler surface, token-resolved caller.
package hub

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
)

// tcpPortMetaKey names the persisted TCP port in the store's meta table.
const tcpPortMetaKey = "agent_tcp_port"

// serveAgentTCP starts the loopback TCP agent channel and records its port so
// Launch can hand it to pods. It reuses the previously-bound port when free, so a
// hub restart keeps the same address and already-running pods stay connected (D11);
// only if that port is taken does it fall back to an ephemeral one.
func (h *Hub) serveAgentTCP() error {
	port := 0
	if v, ok, _ := h.store.GetMeta(tcpPortMetaKey); ok {
		port, _ = strconv.Atoi(v)
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil && port != 0 { // saved port unavailable — take any free one
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return fmt.Errorf("serve agent tcp: %w", err)
	}
	h.agentTCPLn = ln
	h.agentTCPPort = ln.Addr().(*net.TCPAddr).Port
	if err := h.store.SetMeta(tcpPortMetaKey, strconv.Itoa(h.agentTCPPort)); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "hub: agent TCP channel on 127.0.0.1:%d (macOS; token-authenticated)\n", h.agentTCPPort)
	go http.Serve(ln, logRequests("agent-tcp", h.agentTCPHandler()))
	return nil
}

// agentTCPHandler authenticates the bearer token, resolves it to an agent, and
// dispatches to that agent's usual handler — so the token stands in for the unix
// socket's "the endpoint is the identity".
func (h *Hub) agentTCPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project, name, ok := h.agentForToken(r.Header.Get("X-Sindri-Token"))
		if !ok {
			http.Error(w, `{"error":"unauthorized: bad or missing agent token"}`, http.StatusUnauthorized)
			return
		}
		h.agentHandler(project, name).ServeHTTP(w, r)
	})
}
