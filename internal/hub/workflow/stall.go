// package: workflow / stall
// type:    logic (a held assignment nobody is working on)
// job:     recognise an agent that holds work but has stopped moving, and nudge it back onto the
// task it already has — the counterpart to nudging an idle worker toward work it doesn't.
// limits:  the rule and the message; the dwell is measured by the watchdog and the cadence by
// hub/stallwatch.go.
package workflow

import "time"

// StallDwell is how long a working agent must read idle before the hub calls it stalled. Long enough
// that a thinking model, a human reading the pane, or a build under a prompt all expire first.
const StallDwell = 5 * time.Minute

// Stalled reports whether an agent holds work it has stopped doing: a subtask or task in "working",
// or a feature whose subtasks are all checkpointed and which is therefore due to be submitted. The
// phase that is left out is the one that exists to wait — "submitted" awaits a verdict, and waiting
// is the whole of its job. A finished feature used to belong in that group, back when only a human
// could open its milestone PR; now the worker submits it, so sitting there is a stall like any other.
func Stalled(phase, container, runtime string, idleFor time.Duration) bool {
	if runtime != "idle" || idleFor < StallDwell {
		return false
	}
	return phase == "working" || (container != "" && phase != "submitted")
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
	// The subtask if it is on one, else the feature it holds — a worker between subtasks still has
	// something to be getting on with, and naming it is the point of the nudge.
	held := st.Task
	if held == "" {
		held = st.Container
	}
	if held == "" {
		return false // nothing to name, so nothing useful to say
	}
	if !e.deps.AgentAlive(project, name) {
		return false
	}
	if err := e.deps.InjectWhenReady(project, name, MsgStalled(held, idleFor)); err != nil {
		return false
	}
	_ = ps.Log(name, "nudge", "stalled on "+held+" — idle for "+idleFor.Round(time.Minute).String())
	return true
}
