// package: client / messaging
// type:    adapter (hub HTTP client)
// job:     the two ways a message reaches an agent, as a front-end sees them: PUSH it into the
// live session now, or read what is waiting in its mailbox. That pair is the same one
// every sender inside the hub answers (-> workflow.Delivery).
// limits:  transport only; which path a message deserves is its sender's decision, and the
// mailbox itself is the hub's.
package client

import (
	"fmt"

	"github.com/flo-at/sindri/internal/api"
)

// Tell delivers a provenance-stamped message into an agent's session. signedOut is what to do if
// the agent's pane reads signed out: refuse (api.SignedOutRefuse), restart it first, or send
// regardless — the sender's call, since the pane reading may be older than what they know.
func (c *HTTP) Tell(name, msg, source, signedOut string) error {
	return c.post("/tell", api.TellReq{Name: name, Msg: msg, Source: source, SignedOut: signedOut})
}

// MailAgent mails a message to an agent: it waits in the mailbox until the agent reads it, and does
// NOT interrupt what the agent is doing. The counterpart to Tell, which wakes it now and is lost if it
// is not there — mail is what reaches an agent that is down, restarting or signed out.
func (c *HTTP) MailAgent(name, msg string) error {
	return c.post("/agent/mail", api.TellReq{Name: name, Msg: msg, Source: api.SenderUser})
}

// MailBody returns one message from an agent's mailbox with its FULL body. The board carries a
// window of mail with each body cut to a preview, so this is how a detail view shows a message
// whole — and how one older than that window is reached at all.
func (c *HTTP) MailBody(id int64) (api.Mail, error) {
	var m api.Mail
	return m, c.get(fmt.Sprintf("/mail?id=%d", id), &m)
}
