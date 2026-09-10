// package: hub / tick_stalled
// type:    logic (the tick behind a stalled assignment)
// job:     ask the workflow to nudge any agent that holds work but has read idle past the dwell, so
// an agent that stopped mid-assignment is noticed instead of holding a task indefinitely.
// limits:  just the cadence and who to ask about; the rule and the message live in
// hub/flow/roles' stall_act.go, and the dwell is measured by the watchdog.
package hub

import (
	"context"
	"time"
)

// stallInterval is short against the dwell, which is minutes. A TIMER ON PURPOSE: a stall is the
// ABSENCE of an event — a screen that stopped changing — so nothing can announce it. The observer
// answers this when it starts publishing (openspec change the-observer-announces).
const stallInterval = 20 * time.Second

// stallwatch nudges agents that have gone quiet. It holds nothing: prodding once per spell is the
// RULE's own guarantee now, since the worker's map notices a stall too (-> roles.NudgeStalled).
type stallwatch struct{ h *Hub }

// newStallwatch builds the sweep. The registry starts it.
func newStallwatch(h *Hub) *stallwatch { return &stallwatch{h: h} }

// sweep nudges every agent whose held work has gone quiet.
func (s *stallwatch) sweep(context.Context) {
	agents, err := s.h.store.AllAgents()
	if err != nil {
		return
	}
	for _, a := range agents {
		// An idle agent with mail waiting is woken here, on the same tick that catches a stall: an agent
		// that finished and stopped calling `sindri` would otherwise never read what it was sent, which
		// would make "mail must be read" false exactly when it mattered. What it has already been told
		// about is kept per MESSAGE in the mailbox, so a hub restart announces nothing twice.
		s.h.mail.NudgeMailWaiting(a.Project, a.Name)
		obs := s.h.observed(a.Project, a.Name)
		// The INSTANT, which is why this cannot just call StillFor — but the rule choosing between the
		// two clocks is the observation's, asked rather than repeated (-> observe.Observation).
		since := obs.StillSince
		if obs.TurnCutOff() {
			since = obs.StateSince
		}
		if !obs.Seen() || !obs.Up || since.IsZero() {
			s.h.wf.Handles().Prodded.Moving(a.Project + "/" + a.Name) // moving again: the next stall is new
			continue
		}
		s.h.agentFlow().NudgeStalled(a.Project, a.Name, obs, time.Since(since))
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
