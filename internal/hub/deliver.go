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
	"strings"

	"github.com/flo-at/sindri/internal/hub/workflow"
)

// senderOf reads a message's provenance from the tag it already opens with — every message the hub
// injects is stamped "[hub] ", "[user] " or "[reviewer] " (D12), and that stamp is what the agent
// itself sees. Read rather than restated, so a mail row's sender and the line the agent reads cannot
// disagree; anything unstamped is the hub speaking in its own voice.
func senderOf(text string) string {
	if open := strings.IndexByte(text, '['); open == 0 {
		if close := strings.IndexByte(text, ']'); close > 1 {
			switch tag := text[1:close]; tag {
			case "hub", "user", "reviewer":
				return tag
			}
		}
	}
	return "hub"
}

// Deliver sends text to an agent the way d says. MAIL FIRST: a crash between the two loses only the
// wake, which the mail then covers, where the other order loses the message itself.
//
// A push failure is NOT returned — an agent that is down cannot be typed into, and with the mail
// written that is no longer a loss. What is returned is a failure to RECORD.
func (h *Hub) Deliver(project, name, text string, d workflow.Delivery) error {
	if !d.Sends() {
		return fmt.Errorf("delivery to %s/%s asks for neither mail nor push, so it is not a message", project, name)
	}
	ps := h.store.For(project)
	var mailID int64
	if d.Mail {
		m, err := ps.AddMail(name, senderOf(text), text, false)
		if err != nil {
			return err
		}
		mailID = m.ID
		h.notify() // the unread count is on the board
	}
	if !d.Push {
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
	return h.Deliver(project, name, "[user] "+msg, workflow.MailOnly)
}
