// package: hub/workflow / delivery
// type:    logic (how a message reaches an agent)
// job:     the two independent questions every sender answers — must the agent READ this,
// and should it ACT NOW — as one type, plus the three combinations that exist.
// Mail is guaranteed and waits to be read; a push wakes the agent and may be lost.
// limits:  the classification only. Writing the mail and typing into the session is the
// hub's (-> hub.Deliver), and which class a message is belongs to its sender.
package workflow

// Delivery is how one message reaches an agent. Stating both properties is what keeps a message that
// MUST be read off the best-effort path, where an injection into a down agent is simply lost.
//
// No lifetime here on purpose: nothing expires, since a clock cannot tell whether a message is still
// true. Staleness is settled at pickup instead, by pointing the reader at state it can re-read.
type Delivery struct {
	// Mail keeps the message until the agent reads it, whatever the agent is doing meanwhile.
	Mail bool
	// Push types it into the agent's session now. Best-effort by nature: waking is the whole point,
	// so a wake nobody was there to receive is nothing worth keeping.
	Push bool
	// Sender is WHO the message is from — hub, user, reviewer, or an agent by name. STATED, never read
	// out of the text: a prefix in the body made provenance a property of the wording, under which only
	// two of the four could ever be recorded. Empty means the hub in its own voice.
	Sender string
}

// From names the sender, returning a COPY — so one call site cannot leak a sender into the shared
// classification, and reads its provenance beside it: MailAndPush.From(api.SenderUser).
func (d Delivery) From(sender string) Delivery {
	d.Sender = sender
	return d
}

// The three combinations that are messages at all: it must not be missed AND acted on now; it must be
// read but must not interrupt; or its entire value is being live (a nudge fires again next tick).
//
// Marking chatter as mail adds noise PERMANENTLY — nothing is ever deleted — so a new message answers
// "may this be lost?" and "does this deserve to be kept for ever?".
var (
	MailAndPush = Delivery{Mail: true, Push: true}
	MailOnly    = Delivery{Mail: true}
	PushOnly    = Delivery{Push: true}
)

// Sends reports whether this delivery does anything at all. Neither property set is not a message,
// which is a sender's bug rather than a state the hub should quietly accept.
func (d Delivery) Sends() bool { return d.Mail || d.Push }
