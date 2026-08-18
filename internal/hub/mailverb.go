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
	"strings"

	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// mailHelp advertises both halves in one line, since they are one verb: no argument READS, a name SENDS.
const mailHelp = "read what is waiting for you (mail), or send an agent a message (mail <agent> <message>)"

// cmdMail prints an agent's unread messages and marks them read, OLDEST FIRST — a later message often
// supersedes an earlier one. Marked per message as it is printed, so nothing handed over stays unread.
func (h *Hub) cmdMail(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) > 0 {
		return h.sendMail(c, args[0], strings.Join(args[1:], " "), out)
	}
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

// sendMail is `mail <agent> <message...>`: one agent writing to another by BARE NAME, which it was given
// by whoever asked it to make contact — so no roster listing comes with this. MAIL ONLY, never a push:
// agents that can interrupt each other invite a ping-pong nobody asked for.
func (h *Hub) sendMail(c registry.Caller, to, msg string, out io.Writer) (int, error) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		fmt.Fprintln(out, workflow.ReplyMailNoMessage(to))
		return 2, nil
	}
	if n := len([]rune(msg)); n > maxMessageLen {
		fmt.Fprintln(out, workflow.ReplyMailTooLong(n, maxMessageLen))
		return 1, nil
	}
	project, name, err := h.resolveRecipient(to)
	if err != nil {
		fmt.Fprintf(out, "%v\n", err)
		return 1, nil
	}
	if project == c.Project && name == c.Agent {
		fmt.Fprintln(out, workflow.ReplyMailToSelf)
		return 1, nil
	}
	// The sender is QUALIFIED where the address was bare: a message from another repo is useless
	// without knowing which one it came from, and the reply path is then the bare name again.
	from := h.repoName(c.Project) + "/" + c.Agent
	if err := h.Deliver(project, name, msg, workflow.MailOnly.From(from)); err != nil {
		return 1, err
	}
	_ = h.store.For(c.Project).Log(c.Agent, "mail-sent", name+": "+msg)
	fmt.Fprintln(out, workflow.ReplyMailSent(name, h.repoName(project)))
	return 0, nil
}

// resolveRecipient turns what was typed into one mailbox: a bare name looked up fleet-wide, or
// `<repo>/<agent>` naming one directly — never required, but the way through an ambiguity, which is
// REFUSED rather than guessed since uniqueness is only the allocator's convention.
func (h *Hub) resolveRecipient(to string) (project, name string, err error) {
	if repo, agent, ok := strings.Cut(to, "/"); ok {
		matches, merr := h.store.AgentsNamed(agent)
		if merr != nil {
			return "", "", merr
		}
		for _, a := range matches {
			if h.repoName(a.Project) == repo || a.Project == repo {
				return a.Project, a.Name, nil
			}
		}
		return "", "", fmt.Errorf("no agent %q in %q — check the name and the repo", agent, repo)
	}
	matches, err := h.store.AgentsNamed(to)
	if err != nil {
		return "", "", err
	}
	switch len(matches) {
	case 0:
		return "", "", fmt.Errorf("no agent named %q anywhere in the fleet", to)
	case 1:
		return matches[0].Project, matches[0].Name, nil
	}
	qualified := make([]string, len(matches))
	for i, a := range matches {
		qualified[i] = h.repoName(a.Project) + "/" + a.Name
	}
	return "", "", fmt.Errorf("%q is ambiguous — it names %s. Say which with `sindri mail %s <message>`",
		to, strings.Join(qualified, " and "), qualified[0])
}
