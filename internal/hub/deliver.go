// package: hub / deliver
// type:    logic (the one door every message to an agent goes through)
// job:     carry out a classified delivery — write the mail that must be read, push the wake
// that should act now, and record on the mail row whether that wake actually landed.
// limits:  the mechanics of one send. WHICH class a message is belongs to its sender
// (-> workflow.Delivery); the mailbox itself is the store's.
package hub

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/api"

	"github.com/flo-at/sindri/internal/hub/workflow"
)

// maxMessageLen is the longest message an agent may send, to the user or another agent — shared because
// the reason is (brevity serves the reader), where the note grant and fleet ceiling guard the USER's
// attention alone. Over-length is REFUSED, never truncated: a silent cut teaches nothing.
const maxMessageLen = 300

// senderFor is who a message is from: what the sender stated, else the hub in its own voice — which is
// what an unattributed hub message IS, rather than a value to guess at.
func senderFor(d workflow.Delivery) string {
	if d.Sender != "" {
		return d.Sender
	}
	return "hub"
}

// Deliver sends text to an agent the way d says. MAIL FIRST, so a crash between the two loses only the
// wake. A push failure is NOT returned once mail is written — only a failure to RECORD is.
func (h *Hub) Deliver(project, name, text string, d workflow.Delivery) error {
	if !d.Sends() {
		return fmt.Errorf("delivery to %s/%s asks for neither mail nor push, so it is not a message", project, name)
	}
	ps := h.store.For(project)
	var mailID int64
	if d.Mail {
		m, err := ps.AddMail(name, senderFor(d), text, false)
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
	if err := h.agents.InjectWhenReady(project, name, text); err != nil {
		// Push-only had nowhere else to go, so say so where a user reconstructs what an agent was
		// never told. With mail written, the message is not lost and the row's pushed flag stays
		// false, which is what tells a reader it is waiting rather than possibly already acted on.
		if !d.Mail {
			fmt.Fprintf(os.Stderr, "hub: push to %s/%s did not land: %v\n", project, name, err)
		}
		return nil
	}
	if mailID != 0 {
		return ps.MarkMailPushed(mailID)
	}
	return nil
}

// MailAgent puts a user's message in an agent's mailbox and deliberately does NOT push it: choosing
// mail over `tell` IS the choice not to interrupt. It therefore reaches an agent `tell` cannot — down,
// restarting or signed out — since the signed-out refusal belongs to the push path alone.
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
