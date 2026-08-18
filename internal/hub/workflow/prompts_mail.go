// package: hub/workflow / prompts mail
// type:    logic (the agent-facing strings for the mailbox)
// job:     what the hub says about mail — the reminder that some is waiting, and what an
// agent is told when it has read it. Staleness is settled HERE rather than by a
// clock: the reader is sent back to the live state to check each message still holds.
// limits:  pure strings; the mailbox is the store's and the classification the sender's.
package workflow

import "fmt"

// DirUnreadMail is the answer to `sindri` while mail is waiting. It REPLACES the ordinary directive
// rather than sitting beside it, because mail is one-shot consequence — a verdict, a cancellation, an
// edit to the task in hand — and any of those can change what the next action should be. Reading
// first and asking again is therefore the correct order, not an extra round trip.
func DirUnreadMail(n int) string {
	return fmt.Sprintf("[hub] You have %d unread message(s) — things you must read, kept for you "+
		"however busy or away you were when they were sent. Run `sindri mail` to read them (that "+
		"marks them read; nothing is deleted), then `sindri` again for your next action. Read them "+
		"first: a verdict, a cancellation or an edit to the task you hold can change what that action "+
		"is, which is why they are not simply pushed at you and hoped for.", n)
}

// ReplyNoMail answers `mail` with an empty mailbox. It says what the mailbox IS, since "no messages"
// alone reads as a fault to an agent that was told to check.
const ReplyNoMail = "No unread messages. Anything you must read is kept here until you do, so an " +
	"empty mailbox means nothing is waiting — run `sindri` for your next action."

// ReplyMailRead closes a pick-up by settling relevance, which is the one thing a stored message
// cannot settle for itself: nothing expires, so a message is delivered whenever it is read and the
// reader is the only one who can tell whether it still holds. That is why it points at live state
// rather than trusting its own age.
func ReplyMailRead(n int) string {
	return fmt.Sprintf("Marked %d message(s) read — they stay on the record, so a human can still "+
		"see what you were told. Now CHECK EACH ONE against the current state before acting: mail "+
		"never expires, so an older message may already be settled. `sindri` gives your next action, "+
		"`sindri task` the task you hold, `sindri prs` your pull requests. Where a message and the "+
		"live state disagree, the live state wins.", n)
}
