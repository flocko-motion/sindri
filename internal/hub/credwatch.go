// package: hub / credwatch
// type:    logic (the tick behind credential upkeep)
// job:     carry the host's Claude credentials into each agent's home on a slow loop,
// so a re-login on the host reaches pods that are already running.
// limits:  the cadence and lifecycle only; whether a copy is warranted is the agent
// backend's call (-> adapter/agent.RestageCredentials).
package hub

import (
	"log"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/tools/paths"
)

// credInterval paces the upkeep. An access token lasts hours and a re-login is a human act, so a
// few minutes is prompt enough, and each pass is a couple of small file reads per agent.
const credInterval = 5 * time.Minute

// credwatch re-stages agent credentials until stopped; one per hub, started by New.
type credwatch struct {
	h    *Hub
	stop chan struct{}
	done chan struct{}
}

// newCredwatch starts the loop. It must not block: New runs before Serve answers the socket.
func newCredwatch(h *Hub) *credwatch {
	c := &credwatch{h: h, stop: make(chan struct{}), done: make(chan struct{})}
	go c.loop()
	return c
}

func (c *credwatch) loop() {
	defer close(c.done)
	t := time.NewTicker(credInterval)
	defer t.Stop()
	c.sweep()
	for {
		select {
		case <-c.stop:
			return
		case <-t.C:
			c.sweep()
		}
	}
}

// sweep offers every agent the host's credentials. Announced when one is taken, because a token
// changing under a running agent explains behaviour that is otherwise hard to place.
func (c *credwatch) sweep() {
	agents, err := c.h.store.AllAgents()
	if err != nil {
		log.Printf("hub: credential upkeep: roster: %v", err)
		return
	}
	for _, a := range agents {
		wrote, err := agentport.RestageCredentials(paths.AgentHomeDir(a.Project, a.Name))
		if err != nil {
			log.Printf("hub: credential upkeep for %s: %v", a.Name, err)
			continue
		}
		if wrote {
			log.Printf("hub: refreshed %s's Claude credentials from the host", a.Name)
		}
	}
}

// close stops the loop and waits for the sweep in flight.
func (c *credwatch) close() {
	close(c.stop)
	<-c.done
}
