// package: hub / deliver
// type:    logic (the one door every message to an agent goes through)
// job:     carry out a classified delivery — write the mail that must be read, push the wake
// that should act now, and record on the mail row whether that wake actually landed.
// limits:  the mechanics of one send. WHICH class a message is belongs to its sender
// (-> workflow.Delivery); the mailbox itself is the store's.
package hub

import (
	"errors"
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"

	"github.com/flo-at/sindri/internal/hub/workflow"
)

// maxMessageLen is the longest message an agent may send anyone — one limit, since brevity serves the
// reader either way. REFUSED over-length, never truncated: a silent cut teaches nothing.
const maxMessageLen = 300

// errPushDidNotLand answers a push-only delivery the wake gate refused — an injection failure returns
// its own real error instead, so this covers only the case with no underlying cause to report.
var errPushDidNotLand = errors.New("push did not land")

// senderFor is who a message is from: what the sender stated, else the hub in its own voice — which is
// what an unattributed hub message IS, rather than a value to guess at.
func senderFor(d workflow.Delivery) string {
	if d.Sender != "" {
		return d.Sender
	}
	return "hub"
}

// Deliver sends text to an agent the way d says. MAIL FIRST, so a crash between the two loses only the
// wake, which the mailbox then absorbs; push-only, the failure is returned, nothing else saying it.
func (h *Hub) Deliver(project, name, text string, d workflow.Delivery) error {
	if !d.Sends() {
		return fmt.Errorf("delivery to %s/%s asks for neither mail nor push, so it is not a message", project, name)
	}
	ps := h.store.For(project)
	var mailID int64
	if d.Mail {
		m, err := ps.AddMail(name, senderFor(d), text, false, d.ReplyTo)
		if err != nil {
			return err
		}
		mailID = m.ID
		h.notify() // the unread count is on the board
	}
	// The user has no session to type into, so their mailbox IS the channel: a push to them is not a
	// failure to report, it is a thing that does not exist.
	if !d.Push || name == api.SenderUser {
		return nil
	}
	// Mail already carries the message, so refusing here loses only the interruption: the agent reads it
	// on its own next ask — unless d.Unconditional says this push IS the exit from that very state.
	if r := h.wf.WakeRefusal(project, name); r != "" && !d.Unconditional {
		_ = ps.Log(name, "push-suppressed", r+" — not woken for: "+text)
		if !d.Mail {
			return errPushDidNotLand
		}
		return nil
	}
	// The hub's lifetime, not the caller's: a push must land whether or not whoever triggered it is
	// still there. `pushed` means typed, submitted and not contradicted by the pane (-> api.Mail.History).
	if err := h.agents.InjectWhenReady(h.lifetime, project, name, text); err != nil {
		_ = ps.LogMail(mailID, store.MailPushFailed, err.Error())
		// Said whatever the class. Three consecutive failures to one agent left no trace of WHY
		// anywhere — its log records the text as inject-skipped or inject-unconfirmed, never the reason.
		fmt.Fprintf(os.Stderr, "hub: push to %s/%s did not land: %v\n", project, name, err)
		// A push with no mail behind it IS the message, so a swallowed failure reads as delivered —
		// NudgeMailWaiting announced hepti's mailbox off this nil once, and never again. The real cause,
		// not a sentinel: mail written absorbs it, and the announcement retries until it lands.
		if !d.Mail {
			return err
		}
		return nil
	}
	if mailID != 0 {
		_ = ps.LogMail(mailID, store.MailPushLanded, "")
		return ps.MarkMailPushed(mailID)
	}
	return nil
}

// MailAgent mailboxes a user's message and deliberately does NOT push it: choosing mail over `tell` IS
// the choice not to interrupt, so it reaches an agent `tell` cannot — down, restarting or signed out.
func (h *Hub) MailAgent(project, name, msg string) error {
	if _, ok, err := h.store.For(project).GetAgent(name); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	// The "[user] " prefix stays in the rendered line, where a reader wants it, but it is no longer the
	// mechanism: the sender is stated.
	return h.Deliver(project, name, "[user] "+msg, workflow.MailOnly.From(api.SenderUser))
}

// ReplyToMail is the user answering an agent, from either front-end. The recipient comes from the
// stored row: whoever is reading a message should not have to retype who wrote it.
func (h *Hub) ReplyToMail(id int64, msg string) error {
	original, ok, err := h.store.MailByID(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no message %d", id)
	}
	if original.Sender == "hub" || original.Sender == "" {
		return fmt.Errorf("message %d came from the hub, which has nobody behind it to read a reply", id)
	}
	if n := len([]rune(msg)); n > maxMessageLen {
		return fmt.Errorf("that reply is %d characters and the limit is %d — the cost is the agent's context", n, maxMessageLen)
	}
	// The sender is stored qualified (repo/agent) where it came from another repo, so resolve it the
	// same way an agent's reply does rather than assuming the reading repo.
	project, name := original.Project, original.Sender
	if p, n, rerr := h.resolveRecipient(original.Sender); rerr == nil {
		project, name = p, n
	}
	// Mail, not a push: the user chose to answer in writing, and `tell` is the verb that interrupts.
	return h.Deliver(project, name, "[user] "+msg, workflow.MailOnly.From(api.SenderUser).Answering(id))
}
