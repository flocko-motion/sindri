// package: hub / mail verb
// type:    logic (the agent's side of the mailbox)
// job:     `sindri mail` — hand an agent everything it has not read, oldest first, and mark
// those messages read. Pick-up is explicit and it is what survives a crash between
// delivery and processing, which an injection cannot.
// limits:  reading only; who writes mail is the sender's (-> workflow.Delivery), and the
// record is the store's (reading MARKS, it never deletes).
package hub

import (
	"fmt"
	"io"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// mailHelp is what the command registry advertises for the agent's mail verb.
const mailHelp = "read the messages waiting for you, and mark them read: mail"

// cmdMail prints an agent's unread messages and marks them read. Oldest first, because that is the
// order they were sent in and a later message often supersedes an earlier one.
//
// Marking happens per message as it is printed, so a message the agent has been GIVEN is never left
// unread — and because reading only marks, everything stays on the record for a human afterwards.
func (h *Hub) cmdMail(c registry.Caller, _ []string, out io.Writer) (int, error) {
	ps := h.store.For(c.Project)
	unread, err := ps.UnreadMail(c.Agent)
	if err != nil {
		return 1, err
	}
	if len(unread) == 0 {
		fmt.Fprintln(out, workflow.ReplyNoMail)
		return 0, nil
	}
	fmt.Fprintf(out, "%d message(s) waiting, oldest first:\n", len(unread))
	for _, m := range unread {
		fmt.Fprintf(out, "\n── %s · %s ──\n%s\n", m.SentAt, m.Sender, m.Body)
		if err := ps.MarkMailRead(m.ID); err != nil {
			return 1, err
		}
	}
	fmt.Fprintf(out, "\n%s\n", workflow.ReplyMailRead(len(unread)))
	h.notify() // the unread count is on the board, and it has just changed
	return 0, nil
}
