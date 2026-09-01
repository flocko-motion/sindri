// package: hub / credwatch
// type:    logic (the tick behind credential upkeep)
// job:     carry the host's Claude credentials into each agent's home on a slow loop,
// so a re-login on the host reaches pods that are already running.
// limits:  the cadence and lifecycle only; whether a copy is warranted is the agent
// backend's call (-> adapter/agent.RestageCredentials).
package hub

import (
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/tools/paths"
)

// credInterval paces the upkeep. It is the whole fleet's outage window, not a housekeeping cadence:
// every agent runs on ONE shared token, so when it lapses they ALL stop at once and stay stopped
// until the replacement reaches them. Five minutes of that was measured — the token expired at
// 17:45, five agents had rewritten their own credentials blank by 17:43, and the board read them as
// signed out. A pass is a couple of small file reads per agent, so paying it often is cheap.
const credInterval = 15 * time.Second

// expiryLead is how long before the token lapses the hub starts saying so. The renewal is the host's
// Claude Code to make; the hub can only pass it on, so the useful act is naming what is coming.
const expiryLead = 2 * time.Minute

// credwatch re-stages agent credentials until stopped; one per hub, started by New.
type credwatch struct {
	h    *Hub
	stop chan struct{}
	done chan struct{}
	// said is the last thing reported about the host token, so a 15s loop describes a change once
	// rather than filling the log with the same sentence.
	said string
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

// sweep watches the shared token and offers every agent the host's credentials. Announced when one
// is taken, because a token changing under a running agent explains behaviour that is otherwise hard
// to place.
func (c *credwatch) sweep() {
	usable := c.watchToken()
	agents, err := c.h.store.AllAgents()
	if err != nil {
		log.Printf("hub: credential upkeep: roster: %v", err)
		return
	}
	var took, revived []string
	for _, a := range agents {
		wrote, err := agentport.RestageCredentials(paths.AgentHomeDir(a.Project, a.Name))
		if err != nil {
			c.say(fmt.Sprintf("hub: credential upkeep for %s: %v", a.Name, err))
			continue
		}
		if !wrote {
			continue
		}
		took = append(took, a.Name)
		if usable && c.revive(a.Project, a.Name) {
			revived = append(revived, a.Name)
		}
	}
	// One line for the round rather than one per agent: they share a token, so they are refreshed
	// together, and twenty separate lines hide that it was a single event.
	if len(took) > 0 {
		log.Printf("hub: redistributed the host's Claude credentials to %d agent(s): %s",
			len(took), strings.Join(took, ", "))
	}
	// The recovery, not the copy: a person reconstructing a quiet fleet needs to know an agent came
	// back, which the line above never said.
	if len(revived) > 0 {
		log.Printf("hub: restarted %d signed-out agent(s) onto the new credentials: %s",
			len(revived), strings.Join(revived, ", "))
	}
}

// revive restarts an agent that just took a fresh token and is sitting at a /login prompt. Only
// those: the process holds the OLD token in memory, so a new file on disk changes nothing until it
// restarts, and a message typed at that prompt is swallowed (-> agent.Service.Inject's refusal). An
// agent working normally is left alone — a token refreshed under it is routine.
func (c *credwatch) revive(project, name string) bool {
	if !c.stuckAtLogin(project, name) {
		return false
	}
	if err := c.h.agents.RestartAgent(c.h.lifetime, project, name, io.Discard); err != nil {
		log.Printf("hub: restarting signed-out %s onto the new credentials: %v", name, err)
		return false
	}
	return true
}

// stuckAtLogin is the whole rule revive acts on, separated from the acting so it can be checked
// without a runtime. The observer's standing reading, never a probe: this runs over the whole roster.
func (c *credwatch) stuckAtLogin(project, name string) bool {
	o := c.h.observed(project, name)
	return o.Seen() && o.Up && o.SignedOut()
}

// watchToken reports the shared token's state as it changes: gone, lapsed, or about to lapse. Only
// the host can renew it, so the hub's job is to make the coming outage legible rather than let a
// fleet of "signed out" agents be the first news of it.
func (c *credwatch) watchToken() (usable bool) {
	expires, usable := agentport.HostTokenExpiry()
	if !usable {
		c.say("hub: the host has no usable Claude credentials — agents cannot be re-authenticated until you log in on the host")
		return false
	}
	left := time.Until(time.UnixMilli(expires))
	switch {
	case left <= 0:
		c.say(fmt.Sprintf("hub: the shared Claude token lapsed %s ago — every agent is signed out until the host renews it",
			left.Abs().Round(time.Second)))
		// Restarting into a lapsed token returns the agent to the same prompt, which is a loop.
		return false
	case left <= expiryLead:
		c.say(fmt.Sprintf("hub: the shared Claude token lapses in %s; the fleet stops until the host renews it",
			left.Round(time.Second)))
	default:
		c.say("") // healthy: forget the last complaint so the next one is reported afresh
	}
	// Nearly-lapsed still counts: it authenticates now, and an agent brought back for the minutes
	// that remain beats one left at a prompt through them.
	return true
}

// say logs a message once, until it changes.
func (c *credwatch) say(msg string) {
	if msg == c.said {
		return
	}
	c.said = msg
	if msg != "" {
		log.Print(msg)
	}
}

// close stops the loop and waits for the sweep in flight.
func (c *credwatch) close() {
	close(c.stop)
	<-c.done
}
