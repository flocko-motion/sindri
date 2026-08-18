// package: hub/workflow / mail nudge
// type:    logic (waking an agent that has mail waiting)
// job:     tell an idle agent it has unread mail, so "mail must be read" stays true for an agent
// that finished, stopped calling `sindri`, and would otherwise never look again.
// limits:  the rule and the message; the cadence is the hub's (-> hub/stallwatch.go) and the
//
//	mailbox is the store's. Agents gain nothing here: the HUB does the waking.
package workflow

import "fmt"

// idleAndReachable reports an agent a nudge would REACH and that could act on it: alive, at an empty
// prompt, holding nothing, not parked. The empty prompt excludes the states needing a human — blocked,
// signed out, mid-turn — since a nudge none of them can answer is noise on the signal a user watches.
func (e *Engine) idleAndReachable(project, name string) bool {
	st, err := e.store.For(project).GetState(name)
	if err != nil || st.Task != "" || (st.Phase != "" && st.Phase != "idle" && st.Phase != restPhaseFor(project, name, e)) {
		return false
	}
	if !e.deps.AgentAlive(project, name) || !e.deps.AgentIdle(project, name) {
		return false
	}
	return !e.parkedByTheHub(project, name)
}

// restPhaseFor is the phase a role RESTS in — "planning" for a planner, "collab" for a coauthor — so
// resting is not mistaken for holding work. A reviewer and a worker rest in "idle".
func restPhaseFor(project, name string, e *Engine) string {
	a, ok, err := e.store.For(project).GetAgent(name)
	if err != nil || !ok {
		return "idle"
	}
	return restPhase(a.Role)
}

// NudgeMailWaiting wakes an agent that has unread mail, returning the id it nudged for so the caller
// does not repeat itself — what makes the mail guarantee real for an agent that stopped asking. No agent
// gains the power to wake another: the HUB wakes one that has something waiting, which is its job.
//
// Never twice for the same message: an agent told once and still not reading is choosing not to or is
// wedged, and repeating it burns context and teaches it to skim.
func (e *Engine) NudgeMailWaiting(project, name string, lastNudged int64) (int64, bool) {
	ps := e.store.For(project)
	newest, count, err := ps.NewestUnreadMail(name)
	if err != nil || count == 0 || newest == lastNudged {
		return lastNudged, false
	}
	if !e.idleAndReachable(project, name) {
		return lastNudged, false
	}
	if err := e.deps.Deliver(project, name, MsgMailWaiting(count), PushOnly); err != nil {
		return lastNudged, false
	}
	_ = ps.Log(name, "nudge", fmt.Sprintf("%d unread message(s) waiting", count))
	return newest, true
}

// MsgMailWaiting wakes an agent that has stopped asking. Push-only, like every wake: what must be read
// is already kept, and this only prompts the reading.
func MsgMailWaiting(n int) string {
	return fmt.Sprintf("[hub] You have %d unread message(s) waiting — things you must read, sent while "+
		"you were busy or away. Run `sindri mail` to read them, then `sindri` for your next action.", n)
}
