// package: hub/agentchan / tcp
// type:    logic (macOS agent channel over TCP)
// job:     on macOS, where a bind-mounted unix socket can't cross the VM boundary,
//          serve the agent surface over a loopback TCP listener instead,
//          authenticated by a per-agent bearer token. Where it binds and how a pod
//          reaches it come from the wired runtime's AgentChannel (podman vs apple).
// limits:  Linux keeps per-agent unix sockets (agentchan.go); this is the darwin
//          path. Same handler surface, token-resolved caller.
package agentchan

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

// ServeTCP starts the loopback TCP agent channel and records its port so the hub's
// launch path can hand it to pods. It reuses the previously-bound port when free, so
// a hub restart keeps the same address and already-running pods stay connected (D11);
// only if that port is taken does it fall back to an ephemeral one.
func (s *Server) ServeTCP() error {
	// The wired runtime decides where to bind and how a pod addresses the host —
	// podman uses loopback + host.containers.internal, apple container the bridge
	// gateway IP. A failure to determine this is fatal (the channel is unusable).
	ch, err := container.AgentChannel()
	if err != nil {
		return fmt.Errorf("determine agent network channel: %w", err)
	}
	s.dialHost = ch.DialHost
	port := 0
	if v, ok, _ := s.store.GetMeta(tcpPortMetaKey); ok {
		port, _ = strconv.Atoi(v)
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(ch.BindAddr, strconv.Itoa(port)))
	if err != nil && port != 0 { // saved port unavailable — take any free one
		ln, err = net.Listen("tcp", net.JoinHostPort(ch.BindAddr, "0"))
	}
	if err != nil {
		return fmt.Errorf("serve agent tcp on %s: %w", ch.BindAddr, err)
	}
	s.tcpLn = ln
	s.tcpPort = ln.Addr().(*net.TCPAddr).Port
	if err := s.store.SetMeta(tcpPortMetaKey, strconv.Itoa(s.tcpPort)); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "hub: agent TCP channel on %s:%d, pods dial %s (macOS; token-authenticated)\n",
		ch.BindAddr, s.tcpPort, s.dialHost)
	go http.Serve(ln, s.deps.LogRequests("agent-tcp", s.tcpHandler()))
	return nil
}

// Port is the bound TCP port (the hub hands it to pods at launch); 0 until ServeTCP.
func (s *Server) Port() int { return s.tcpPort }

// DialHost is how a pod addresses the host for the TCP channel (runtime-specific);
// "" until ServeTCP.
func (s *Server) DialHost() string { return s.dialHost }

// tcpHandler authenticates the bearer token, resolves it to an agent, and dispatches
// to that agent's usual handler — so the token stands in for the unix socket's "the
// endpoint is the identity".
func (s *Server) tcpHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		project, name, ok, err := s.deps.TokenAgent(r.Header.Get("X-Sindri-Token"))
		if err != nil { // a store failure is 500, not "bad token" — and it's logged, not swallowed
			log.Printf("agent-tcp: token lookup failed: %v", err)
			http.Error(w, `{"error":"token lookup failed"}`, http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, `{"error":"unauthorized: bad or missing agent token"}`, http.StatusUnauthorized)
			return
		}
		s.handler(project, name).ServeHTTP(w, r)
	})
}
