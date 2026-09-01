// package: hub / stallwatch
// type:    logic (the tick behind a stalled assignment)
// job:     ask the workflow to nudge any agent that holds work but has read idle past the dwell, so
// an agent that stopped mid-assignment is noticed instead of holding a task indefinitely.
// limits:  just the cadence and who to ask about; the rule and the message live in
// workflow/stall.go, and the dwell is measured by the watchdog.
package hub

import (
	"time"
)

// stallInterval only has to be short against the dwell, which is minutes: a stall that has already
// lasted that long is not made worse by being found a few seconds later.
const stallInterval = 20 * time.Second

// stallwatch nudges stalled agents until stopped; one per hub, started by New.
type stallwatch struct {
	h    *Hub
	stop chan struct{}
	done chan struct{}
	// nudged records the idle spell each agent was last nudged for, keyed by agent. Keyed on the
	// spell rather than a timestamp so a stall is prodded once — and a NEW stall, which starts a
	// new spell, is prodded again.
	nudged map[agentKey]time.Time
}

// newStallwatch starts the loop. It must not block: New runs before Serve answers the socket.
func newStallwatch(h *Hub) *stallwatch {
	s := &stallwatch{h: h, stop: make(chan struct{}), done: make(chan struct{}),
		nudged: map[agentKey]time.Time{}}
	go s.loop()
	return s
}

// loop sweeps the fleet until stopped.
func (s *stallwatch) loop() {
	defer close(s.done)
	t := time.NewTicker(stallInterval)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.sweep()
		}
	}
}

// close stops the loop and waits for the sweep in flight.
func (s *stallwatch) close() {
	close(s.stop)
	<-s.done
}

// sweep nudges every agent whose held work has gone quiet.
func (s *stallwatch) sweep() {
	agents, err := s.h.store.AllAgents()
	if err != nil {
		return
	}
	for _, a := range agents {
		key := agentKey{a.Project, a.Name}
		// An idle agent with mail waiting is woken here, on the same tick that catches a stall: an agent
		// that finished and stopped calling `sindri` would otherwise never read what it was sent, which
		// would make "mail must be read" false exactly when it mattered. What it has already been told
		// about is kept per MESSAGE in the mailbox, not in this map, so a hub restart announces nothing
		// twice (-> workflow.NudgeMailWaiting).
		s.h.wf.NudgeMailWaiting(a.Project, a.Name)
		obs := s.h.observed(a.Project, a.Name)
		// The spell is keyed on whichever clock this state is judged by, so a cut-off turn that
		// resumes and dies again is a new spell rather than one already prodded for. The INSTANT, which
		// is why this cannot just call StillFor — but the rule choosing between the two clocks is the
		// observation's, asked rather than repeated (-> observe.Observation.StillFor).
		since := obs.StillSince
		if obs.TurnCutOff() {
			since = obs.StateSince
		}
		if !obs.Seen() || !obs.Up || since.IsZero() {
			delete(s.nudged, key) // moving again (or gone): the next stall is a new one
			continue
		}
		if s.nudged[key].Equal(since) {
			continue // already prodded for this spell
		}
		if s.h.wf.NudgeStalled(a.Project, a.Name, obs, time.Since(since)) {
			s.nudged[key] = since
		}
	}
}

// stalledFor is how long an agent's screen has stood still, and whether that counts as stalled — the
// dwell measured here, the verdict asked of the surface, so what the user sees and what the agent is
// told cannot disagree.
func (h *Hub) stalledFor(project, name string) (time.Duration, bool) {
	s, err := h.sit.Of(project, name)
	if err != nil || s.StillFor == 0 {
		return 0, false
	}
	return s.StillFor, s.Allowed().Stalled
}
