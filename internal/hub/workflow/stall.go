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

// Stalled reports whether an agent holds work it has stopped doing. Only phase "working" qualifies,
// which is what excludes the phases that exist to wait: "submitted" awaits a verdict, and an
// all-checkpointed container awaits its next subtask (-> DirContainerWait). Neither reports working.
func Stalled(phase, runtime string, idleFor time.Duration) bool {
	return phase == "working" && runtime == "idle" && idleFor >= StallDwell
}

// NudgeStalled prods an agent holding work to continue or say what blocks it, reporting whether it
// sent anything. The phase is re-read rather than trusted: the dwell is minutes old by definition,
// so the agent may have moved on while it elapsed.
func (e *Engine) NudgeStalled(project, name, runtime string, idleFor time.Duration) bool {
	ps := e.store.For(project)
	st, err := ps.GetState(name)
	if err != nil || !Stalled(st.Phase, runtime, idleFor) {
		return false
	}
	if st.Task == "" {
		return false // nothing to name, so nothing useful to say
	}
	if !e.deps.AgentAlive(project, name) {
		return false
	}
	if err := e.deps.InjectWhenReady(project, name, MsgStalled(st.Task, idleFor)); err != nil {
		return false
	}
	_ = ps.Log(name, "nudge", "stalled on "+st.Task+" — idle for "+idleFor.Round(time.Minute).String())
	return true
}
