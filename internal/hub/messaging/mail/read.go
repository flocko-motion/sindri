// package: hub/messaging/mail / read
// type:    logic (what the user reads, and the one act that retires it)
// job:     hand a front-end one message in full, and mark the user's mail read — the single
// deliberate gesture, and the whole backlog at once.
// limits:  the user's side. An AGENT's unread mail is what it has yet to be told, so no gesture
// here may retire it (-> verb.go, where reading MARKS).
package mail

import (
	"github.com/flo-at/sindri/internal/api"
)

// MailBody returns one message with its full body — what a detail view or `mail show` asks for. A
// PURE read: marking is a separate, deliberate act (-> MarkMailReadForUser), not a side effect of a look.
func (b *Box) MailBody(id int64) (api.Mail, bool, error) {
	m, ok, err := b.store.MailByID(id)
	if err != nil || !ok {
		return m, ok, err
	}
	// The lifecycle rides along here and nowhere else: a listing wants the state, and only somebody
	// asking about ONE message is asking what became of it (-> api.Mail.History).
	m.History, _ = b.store.For(m.Project).MailEvents(id)
	return m, true, nil
}

// MarkMailReadForUser marks one message read, but ONLY when addressed to the user — the one
// deliberate act (a dwell, an ENTER, `mail show`) that may retire a message from the Mail tab.
func (b *Box) MarkMailReadForUser(id int64) error {
	m, ok, err := b.store.MailByID(id)
	if err != nil || !ok || m.Read() || !api.MailToUser(m) {
		return err
	}
	if err := b.store.For(m.Project).MarkMailRead(id); err != nil {
		return err
	}
	b.deps.Notify()
	return nil
}

// MarkAllUserMailRead retires the user's whole unread backlog at once, and returns how many. Same
// scope guarantee as the single-message act: an AGENT's unread mail is what it has yet to be told,
// so no front-end gesture may retire it (-> MarkMailReadForUser).
func (b *Box) MarkAllUserMailRead() (int, error) {
	n, err := b.store.MarkAllUserMailRead()
	if err != nil {
		return 0, err
	}
	if n > 0 {
		b.deps.Notify()
	}
	return n, nil
}
