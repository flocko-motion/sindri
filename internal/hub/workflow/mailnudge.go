// package: hub/workflow / mail nudge
// type:    logic (waking an agent that has mail waiting)
// job:     tell an idle agent it has unread mail, so "mail must be read" stays true for an agent
// that finished, stopped calling `sindri`, and would otherwise never look again.
// limits:  the rule and the message; the cadence is the hub's (-> hub/stallwatch.go) and the
// mailbox is the store's. Agents gain nothing here: the HUB does the waking.
package workflow

import "fmt"

// idleAndReachable reports an agent a nudge would REACH and that could act on it: alive, at an empty
// prompt, holding nothing, not parked. The empty prompt excludes the states needing a human — blocked,
// signed out, mid-turn — since a nudge none of them can answer is noise on the signal a user watches.
func (e *Engine) idleAndReachable(project, name string) bool {
	st, err := e.store.For(project).GetState(name)
	if err != nil || st.Task != "" || (st.Phase != "" && st.Phase != "idle" && st.Phase != e.restPhaseFor(project, name)) {
		return false
	}
	if !e.deps.AgentAlive(project, name) || !e.deps.AgentIdle(project, name) {
		return false
	}
	return !e.parkedByTheHub(project, name)
}

// restPhaseFor is the phase a role RESTS in — "planning" for a planner, "collab" for a coauthor — so
// resting is not mistaken for holding work. A reviewer and a worker rest in "idle".
func (e *Engine) restPhaseFor(project, name string) string {
	a, ok, err := e.store.For(project).GetAgent(name)
	if err != nil || !ok {
		return "idle"
	}
	return restPhase(a.Role)
}

// NudgeMailWaiting wakes an agent that has unread mail it has not been told about, whatever its ROLE — a
// worker asks constantly, while a planner mid-conversation and a reviewer between verdicts can go hours
// without asking, so a task-keyed wake misses exactly the roles that need one. No agent gains the power
// to wake another; the HUB wakes one that has something waiting. Never twice for the same message, and
// marked only after the push LANDS: marking first would leave a message announced to nobody.
func (e *Engine) NudgeMailWaiting(project, name string) bool {
	ps := e.store.For(project)
	unannounced, unread, err := ps.UnannouncedMail(name)
	if err != nil || unannounced == 0 {
		return false
	}
	if !e.idleAndReachable(project, name) {
		return false
	}
	// The message states the WHOLE unread count, not just the new part: what the agent has to deal with
	// is its mailbox, and "1 waiting" beside ten it never read would read as nine having gone away.
	if err := e.deps.Deliver(project, name, MsgMailWaiting(unread), PushOnly); err != nil {
		return false
	}
	if err := ps.MarkMailAnnounced(name); err != nil {
		return false
	}
	_ = ps.Log(name, "nudge", fmt.Sprintf("%d unread message(s) waiting, %d newly announced", unread, unannounced))
	return true
}

// MsgMailWaiting wakes an agent that has stopped asking. Push-only, like every wake: what must be read
// is already kept, and this only prompts the reading.
func MsgMailWaiting(n int) string {
	return fmt.Sprintf("[hub] You have %d unread message(s) waiting — things you must read, sent while "+
		"you were busy or away. Run `sindri mail` to read them, then `sindri` for your next action.", n)
}
