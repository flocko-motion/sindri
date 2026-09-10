// package: hub/flow/agent / stall
// type:    logic (a held assignment nobody is working on)
// job:     recognise an agent that holds work but has stopped moving, and nudge it back onto the
// task it already has — the counterpart to nudging an idle worker toward work it doesn't.
// limits:  the rule and the message; the dwell is measured by the watchdog and the cadence by
// hub/stallwatch.go.
package agent

import (
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"time"

	"github.com/flo-at/sindri/internal/hub/observe"
	"github.com/flo-at/sindri/internal/hub/situation"
)

// StallDwell and RetryDwell are the surface's, re-exported under the names this package's callers
// already use (-> situation.StallDwell).
const (
	StallDwell = situation.StallDwell
	RetryDwell = situation.RetryDwell
)

// NudgeStalled prods an agent holding work to continue or say what blocks it, reporting whether it
// sent anything. It takes the OBSERVATION the caller judged on rather than a word off it: the caller
// measured the dwell against that reading, and re-gathering here would judge one and prod on another.
//
// ONCE PER SPELL, and the register is here rather than at a caller because there are two of them —
// the worker's own map, which re-looks every minute, and the fleet sweep that catches every other
// role. Held by the caller, a stalled worker was prodded once a minute for as long as it stood still.
func (a *Act) NudgeStalled(project, name string, obs observe.Observation, idleFor time.Duration) bool {
	ps := a.Store.For(project)
	// The spell is keyed on whichever clock this state is judged by, so a cut-off turn that resumes
	// and dies again is a new spell rather than one already prodded for (-> observe.Observation).
	spell := obs.StillSince
	if obs.TurnCutOff() {
		spell = obs.StateSince
	}
	// A reading with no spell on it is one nobody has timed — the caller measured the dwell itself
	// and asked, so there is nothing to dedupe against and refusing would swallow the nudge.
	if !spell.IsZero() && !a.Prodded.First(project+"/"+name, spell) {
		return false
	}
	st, err := ps.GetState(name)
	if err != nil {
		return false
	}
	s, err := a.Sit.Of(project, name)
	if err != nil {
		return false
	}
	s.Observation, s.StillFor = obs, idleFor
	allowed := s.Allowed()
	if !allowed.Stalled {
		return false
	}
	// Escalated is idle BY INSTRUCTION, like the parked states below (-> ParkedByTheHub) — but ahead
	// of the api-error retry, since a resumed turn has no verb left that lands work.
	if st.Escalation != "" {
		return false
	}
	if !s.Up { // the watchdog's reading: this runs on the stall tick
		return false
	}
	// Ungated, and ahead of every parked check: finishing a turn already in flight is not new work, so
	// it must reach the agent whatever else is true of it (-> ParkedByTheHub's own exception, below).
	if s.TurnCutOff() {
		if err := a.Harness.Say(project, name, prompts.MsgRetryTurn, mail.PushOnly); err != nil {
			return false
		}
		_ = ps.Log(name, "nudge", "api error cut the turn off — asked it to resume")
		return true
	}
	// Past the api-error retry, not before it: a parked agent is idle BY INSTRUCTION, so prodding it
	// complains about the state the hub put it in. A cut-off turn still deserves resuming.
	if s.ParkedByTheHub() {
		return false
	}
	// Asked directly and quietly: Deliver's own gate would log "push-suppressed" on every stall tick,
	// and an already-declined nudge is not the caller mistake that log exists to catch.
	if allowed.Wake != "" {
		return false
	}
	// The subtask if it is on one, else the feature it holds — a worker between subtasks still has
	// something to be getting on with, and naming it is the point of the nudge. A reviewer's hold is
	// its review, and an authored PR still to land is all that is left once a checkpoint has emptied
	// the state row: austri sat on a rejected pr-sd-a47b61 unnamed.
	held := firstOf(st.Task, st.Container, s.ReviewingPR, s.AwaitingPR)
	if held == "" {
		return false // nothing to name, so nothing useful to say
	}
	if err := a.Harness.Say(project, name, prompts.MsgStalled(held, idleFor), mail.PushOnly); err != nil {
		return false
	}
	_ = ps.Log(name, "nudge", "stalled on "+held+" — idle for "+idleFor.Round(time.Minute).String())
	return true
}

// firstOf is the first non-empty of these, for a message that has to name ONE thing.
func firstOf(ids ...string) string {
	for _, id := range ids {
		if id != "" {
			return id
		}
	}
	return ""
}
