// package: hub/messaging/mail / verb
// type:    logic (the agent's side of the mailbox)
// job:     `sindri mail` — hand an agent everything it has not read, oldest first, and mark
// those messages read; `mail list` for a scoped look at the project's mailbox without
// marking anything. Pick-up is explicit and it is what survives a crash between
// delivery and processing, which an injection cannot.
// limits:  reading only; who writes mail is the sender's (-> Delivery), and the
// record is the store's (reading MARKS, it never deletes).
package mail

import (
	"fmt"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"io"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
)

// Help advertises all three in one line, since they are one verb: no argument READS, a name
// SENDS, and "list" is the one word reserved out of the name position (-> CmdMail).
const Help = "read what is waiting for you (mail), send an agent a message (mail <agent> <message>), " +
	"or list recent mail (mail list)"

// CmdMail prints an agent's unread messages and marks them read, OLDEST FIRST — a later message often
// supersedes an earlier one. Marked per message as it is printed, so nothing handed over stays unread.
func (b *Box) CmdMail(c registry.Caller, args []string, out io.Writer) (int, error) {
	// "list" is reserved ahead of send, mirroring CmdTasks (-> hub/flow/task); nothing else is.
	if len(args) > 0 && args[0] == "list" {
		return b.list(c, out)
	}
	if len(args) > 0 {
		return b.send(c, args[0], strings.Join(args[1:], " "), out)
	}
	ps := b.store.For(c.Project)
	unread, err := ps.UnreadMail(c.Agent)
	if err != nil {
		return 1, err
	}
	if len(unread) == 0 {
		fmt.Fprintln(out, prompts.ReplyNoMail)
		return 0, nil
	}
	fmt.Fprintf(out, "%d message(s) waiting, oldest first:\n", len(unread))
	for _, m := range unread {
		// The id is on the header because an agent that wants to answer needs it, and reading is where
		// it learns one — nothing else hands it out (-> CmdReply).
		fmt.Fprintf(out, "\n── %s · %s · %s ──\n%s\n", api.MailID(m.ID), m.SentAt, m.Sender, m.Body)
		if err := ps.MarkMailRead(m.ID); err != nil {
			return 1, err
		}
	}
	fmt.Fprintf(out, "\n%s\n", prompts.ReplyMailRead(len(unread)))
	b.deps.Notify() // the unread count is on the board, and it has just changed
	b.deps.MailArrived(c.Project, c.Agent)
	return 0, nil
}

// list is "mail list": planner/coauthor see the whole project, worker/reviewer only their own.
// ACTIVE by default — mail is never deleted, so an unfiltered dump only grows (-> api.MailActive).
func (b *Box) list(c registry.Caller, out io.Writer) (int, error) {
	all, err := b.store.For(c.Project).Mail()
	if err != nil {
		return 1, err
	}
	visible := all
	if c.Role == "worker" || c.Role == "reviewer" {
		self := b.deps.RepoName(c.Project) + "/" + c.Agent
		visible = make([]api.Mail, 0, len(all))
		for _, m := range all {
			if m.Agent == c.Agent || m.Sender == self {
				visible = append(visible, m)
			}
		}
	}
	active := api.FilterMail(api.MailActive, "", visible)
	if len(active) == 0 {
		fmt.Fprintln(out, "no active mail — `mail` reads what's unread for you.")
		return 0, nil
	}
	for _, m := range active {
		mark := " "
		if m.Read() {
			mark = "r"
		}
		fmt.Fprintf(out, "%s %-14s %-16s → %-16s %s\n", mark, api.MailID(m.ID), m.Sender, m.Agent, m.SentAt)
	}
	scope := "in the project"
	if c.Role == "worker" || c.Role == "reviewer" {
		scope = "of yours"
	}
	fmt.Fprintf(out, "\n%d active message(s) %s, %d in total — `mail` still reads what's unread for you.\n",
		len(active), scope, len(visible))
	return 0, nil
}

// send sends by bare name, MAIL ONLY never a push — interrupting agents invites unwanted ping-pong.
func (b *Box) send(c registry.Caller, to, msg string, out io.Writer) (int, error) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		fmt.Fprintln(out, prompts.ReplyMailNoMessage(to))
		return 2, nil
	}
	project, name, err := b.Recipient(to)
	if err != nil {
		fmt.Fprintf(out, "%v\n", err)
		return 1, nil
	}
	if project == c.Project && name == c.Agent {
		fmt.Fprintln(out, prompts.ReplyMailToSelf)
		return 1, nil
	}
	// The sender is QUALIFIED where the address was bare: a message from another repo is useless
	// without knowing which one it came from, and the reply path is then the bare name again.
	from := b.deps.RepoName(c.Project) + "/" + c.Agent
	if err := b.Deliver(project, name, msg, MailOnly.From(from)); err != nil {
		return 1, err
	}
	_ = b.store.For(c.Project).Log(c.Agent, "mail-sent", name+": "+msg)
	fmt.Fprintln(out, prompts.ReplyMailSent(name, b.deps.RepoName(project)))
	return 0, nil
}

// ReplyHelp is what the registry advertises for reply.
const ReplyHelp = "answer a message you were sent, without needing to know who sent it: reply <mail-id> <message>"

// CmdReply answers by mail id; the recipient comes from the stored row, so no name is ever needed.
func (b *Box) CmdReply(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) < 2 {
		fmt.Fprintln(out, prompts.ReplyReplyUsage)
		return 2, nil
	}
	id, err := api.ParseMailID(args[0])
	if err != nil {
		fmt.Fprintf(out, "%v — `sindri mail` lists what is waiting, each line with its id.\n", err)
		return 2, nil
	}
	msg := strings.TrimSpace(strings.Join(args[1:], " "))
	if msg == "" {
		fmt.Fprintln(out, prompts.ReplyReplyUsage)
		return 2, nil
	}
	original, ok, merr := b.store.MailByID(id)
	if merr != nil {
		return 1, merr
	}
	// Yours to answer: a reply to somebody else's mail would be a message they never see the question
	// for, and reading another agent's mailbox is not what the id is for.
	if !ok || original.Project != c.Project || original.Agent != c.Agent {
		fmt.Fprintf(out, "No message %s in your mailbox — `sindri mail` shows what you were sent.\n", api.MailID(id))
		return 1, nil
	}
	if original.Sender == "hub" || original.Sender == "" {
		fmt.Fprintln(out, prompts.ReplyReplyToHub)
		return 1, nil
	}
	project, name := c.Project, original.Sender
	if original.Sender == api.SenderUser {
		// Exempt from the note budget: that bounds unprompted attention, not an answer to a message they sent.
		name = api.SenderUser
	} else if p, n, rerr := b.Recipient(original.Sender); rerr == nil {
		project, name = p, n
	} else {
		fmt.Fprintf(out, "%v\n", rerr)
		return 1, nil
	}
	from := b.deps.RepoName(c.Project) + "/" + c.Agent
	if err := b.Deliver(project, name, msg, MailOnly.From(from).Answering(id)); err != nil {
		return 1, err
	}
	_ = b.store.For(c.Project).Log(c.Agent, "mail-reply", fmt.Sprintf("%d to %s: %s", id, name, msg))
	fmt.Fprintln(out, prompts.ReplyReplied(original.Sender, api.MailID(id)))
	return 0, nil
}
