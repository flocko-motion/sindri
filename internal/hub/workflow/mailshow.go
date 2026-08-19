// package: hub/workflow / mail show
// type:    logic (the agent-facing "show <mail-id>" read)
// job:     read one message by id from ANY mailbox, for `show` — deliberately wider than `mail
// list`'s own scoping, since a mail id is unguessable and the case this exists for is a
// human handing an agent a specific id to read regardless of who it was addressed to.
// limits:  a read, never a write: unlike `mail` and `reply`, it must NOT mark the message read —
// that guarantee belongs to the true recipient's own mail-reading path alone.
package workflow

import (
	"fmt"
	"io"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/registry"
)

// CmdShowMail prints one message by id, from any project's mailbox, to whoever asks — a wider
// grant than `mail list` gives on purpose. Recorded here because it looks like a leak to a later
// reader otherwise: an id is unguessable, so the only way an agent not addressed by a message ever
// reaches its id is a human passing it one directly, and reading it back is exactly that case.
//
// It never marks the message read. Doing so on behalf of whoever it was actually sent to would
// silently consume their delivery guarantee before they ever saw it — the same reasoning that
// keeps Hub.MailBody from marking an agent's own mail read on a human's look (-> state.go).
func (e *Engine) CmdShowMail(c registry.Caller, args []string, out io.Writer) (int, error) {
	id, err := api.ParseMailID(args[0])
	if err != nil {
		fmt.Fprintf(out, "%v\n", err)
		return 2, nil
	}
	m, ok, err := e.store.MailByID(id)
	if err != nil {
		return 1, err
	}
	if !ok {
		fmt.Fprintf(out, "no such message %s\n", api.MailID(id))
		return 1, nil
	}
	fmt.Fprintf(out, "%s  %s → %s  %s\n%s\n", api.MailID(m.ID), m.Sender, m.Agent, m.SentAt, m.Body)
	return 0, nil
}
