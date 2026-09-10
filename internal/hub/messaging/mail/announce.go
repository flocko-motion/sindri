// package: hub/messaging/mail / announce
// type:    logic (waking an agent that has mail waiting)
// job:     tell an idle agent it has unread mail, so "mail must be read" stays true for an agent
// that finished, stopped calling `sindri`, and would otherwise never look again.
// limits:  the rule and the message; the cadence is the hub's (-> hub/stallwatch.go) and the
// mailbox is the store's. Agents gain nothing here: the HUB does the waking.
package mail

import (
	"fmt"
	"time"
)

// ReannounceAfter is how long unread mail waits before the agent is told again. A BACKSTOP over the
// SUBSCRIBER — a push send-keys accepted and never delivered — so retiring it needs at-least-once.
const ReannounceAfter = 5 * time.Minute

// NudgeMailWaiting wakes an agent that has unread mail it has not been told about, whatever its ROLE — a
// worker asks constantly, while a planner mid-conversation and a reviewer between verdicts can go hours
// without asking, so a task-keyed wake misses exactly the roles that need one. No agent gains the power
// to wake another; the HUB wakes one that has something waiting. Never twice for the same message, and
// marked only after the push LANDS: marking first would leave a message announced to nobody.
func (b *Box) NudgeMailWaiting(project, name string) bool {
	ps := b.store.For(project)
	unannounced, unread, err := ps.UnannouncedMail(name, time.Now().Add(-ReannounceAfter))
	if err != nil || unannounced == 0 {
		return false
	}
	// Parked stays exempt: retirement and a full context are states the hub itself put the agent in
	// and told it to wait in, and "hands off every automatic behaviour" is the whole of what retiring
	// means. Holding work is NOT such a state, which is the distinction this used to miss.
	if !b.deps.Reachable(project, name) {
		return false
	}
	if !b.deps.MayWake(project, name) {
		return false
	}
	// The whole unread count, not just the new part. Ungated: this push IS the exit from a refusing
	// state (escalated, waiting on exactly this answer), not news of more work.
	if err := b.Deliver(project, name, MsgMailWaiting(unread), PushOnly); err != nil {
		return false
	}
	if err := ps.MarkMailAnnounced(name); err != nil {
		return false
	}
	_ = ps.Log(name, "nudge", fmt.Sprintf("%d unread message(s) waiting, %d newly announced", unread, unannounced))
	return true
}

// MsgMailWaiting wakes an agent that has stopped asking. Push-only, like every wake: what must be read
// is already kept, and this only prompts the reading. Points at the plain verb, not `sindri mail`
// then `sindri`: one ask now delivers the mail and the directive together (-> serveMail).
func MsgMailWaiting(n int) string {
	return fmt.Sprintf("[hub] You have %d unread message(s) waiting — things you must read, sent while "+
		"you were busy or away. Run `sindri` — it reads them and answers your next action in one call.", n)
}
