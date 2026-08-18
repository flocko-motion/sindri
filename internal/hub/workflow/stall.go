// package: workflow / stall
// type:    logic (a held assignment nobody is working on)
// job:     recognise an agent that holds work but has stopped moving, and nudge it back onto the
// task it already has — the counterpart to nudging an idle worker toward work it doesn't.
// limits:  the rule and the message; the dwell is measured by the watchdog and the cadence by
// hub/stallwatch.go.
package workflow

import "time"

// StallDwell is how long an agent's screen must stand completely still before the hub calls it
// stalled. It has to outlast an ordinary build or test run (a tool call froze one pane for 12s+).
const StallDwell = 3 * time.Minute

// RetryDwell is how long a cut-off turn is left before the agent is told to resume. Short because the
// pane STATES the failure, and long enough only that a retry already in flight finishes first.
const RetryDwell = time.Minute

// parkedByTheHub reports whether an agent is idle because it was told to be — retired by a human, or
// by its own context filling. Both are wound down deliberately (-> claimNext).
func (e *Engine) parkedByTheHub(project, name string) bool {
	return e.retired(project, name) || e.ContextFull(project, name)
}

// Stalled reports whether an agent holds work it has stopped doing. The evidence is the SCREEN
// standing still — a pane frozen mid-turn keeps SAYING "working" forever. Two words still veto it,
// both meaning the agent is correctly motionless: "blocked" waits on a human, "signed-out" cannot
// act. Which work counts: "working", or a feature due to be submitted; "submitted" and "gating"
// (a queued gate result pending) both exist to wait.
func Stalled(phase, container, runtime string, stillFor time.Duration) bool {
	// A cut-off turn counts in ANY phase: nothing resumes on its own, and an agent that could not
	// finish its own sentence will not act on a verdict either.
	if runtime == "api-error" {
		return stillFor >= RetryDwell
	}
	if runtime == "blocked" || runtime == "signed-out" || stillFor < StallDwell {
		return false
	}
	return phase == "working" || (container != "" && phase != "submitted" && phase != "gating")
}

// NudgeStalled prods an agent holding work to continue or say what blocks it, reporting whether it
// sent anything. The phase is re-read rather than trusted: the dwell is minutes old by definition,
// so the agent may have moved on while it elapsed.
func (e *Engine) NudgeStalled(project, name, runtime string, idleFor time.Duration) bool {
	ps := e.store.For(project)
	st, err := ps.GetState(name)
	if err != nil || !Stalled(st.Phase, st.Container, runtime, idleFor) {
		return false
	}
	// Escalated is idle BY INSTRUCTION, like the parked states below (-> parkedByTheHub) — but ahead
	// of the api-error retry, since a resumed turn has no verb left that lands work.
	if st.Escalation != "" {
		return false
	}
	if !e.deps.AgentAlive(project, name) {
		return false
	}
	// A cut-off turn is answered on its own terms: it is not idling and has nothing to explain, it
	// simply stopped mid-sentence. Sent whatever it holds, since the retry is about the turn.
	if runtime == "api-error" {
		if err := e.deps.Deliver(project, name, MsgRetryTurn, PushOnly); err != nil {
			return false
		}
		_ = ps.Log(name, "nudge", "api error cut the turn off — asked it to resume")
		return true
	}
	// Past the api-error retry, not before it: a parked agent is idle BY INSTRUCTION — DirFull tells
	// it "do not ask again, just wait" — so prodding it complains about the one state the hub put it
	// in. A turn cut off mid-sentence is a different thing, and still deserves resuming.
	if e.parkedByTheHub(project, name) {
		return false
	}
	// The subtask if it is on one, else the feature it holds — a worker between subtasks still has
	// something to be getting on with, and naming it is the point of the nudge.
	held := st.Task
	if held == "" {
		held = st.Container
	}
	if held == "" {
		return false // nothing to name, so nothing useful to say
	}
	if err := e.deps.Deliver(project, name, MsgStalled(held, idleFor), PushOnly); err != nil {
		return false
	}
	_ = ps.Log(name, "nudge", "stalled on "+held+" — idle for "+idleFor.Round(time.Minute).String())
	return true
}
