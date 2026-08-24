// package: ui/cli / mail
// type:    command (host CLI)
// job:     the `sindri mail` group — list what agents have been sent and must read, across
// the fleet, filtered by recipient and by unread; and show one message in full.
// limits:  no logic — the rows come off the board and the filters are api's, so this
// listing and the TUI's Mail tab can only ever show the same set.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/table"
	"github.com/spf13/cobra"
)

// NewMailCmd builds the `mail` command tree (what agents have been told and must read).
func NewMailCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "mail",
		Short: "What agents have been sent and must read (mail is kept, reading only marks it)",
		Long: "An agent's mailbox holds the messages it MUST read — a verdict, a rejection with its\n" +
			"feedback, an assignment. Reading one marks it read; nothing is ever deleted, so this is\n" +
			"the record of what an agent was told, not a queue you are watching drain.\n\n" +
			"Push-only traffic (a stall nudge, a meeting broadcast) is not here by design: waking the\n" +
			"agent is its entire purpose, and it is recorded per agent in `sindri agent info`.\n\n" +
			"Your OWN mail works the same way: `mail show` marks a message read the moment you ask for\n" +
			"it. The TUI's Mail tab does too, but only once the cursor has rested on one for a few\n" +
			"seconds with its body on screen — moving the cursor alone never marks anything.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	c.AddCommand(mailListCmd(), mailShowCmd(), mailReplyCmd())
	return c
}

func mailListCmd() *cobra.Command {
	var agent, filter string
	var mine bool
	var limit int
	c := &cobra.Command{
		Use: "list", Short: "List mail across the fleet, newest first", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			f, err := api.ParseMailFilter(filter)
			if err != nil {
				return err
			}
			if mine { // the reservation is not something a user should have to know to type
				agent = api.SenderUser
			}
			return withBackend(func(b backend) error {
				st, err := b.State()
				if err != nil {
					return err
				}
				// AllMail orders newest first, so capHead keeps that end.
				rows, matched := capHead(api.FilterMail(f, agent, st.Mail), limit)
				// Grouped the way every fleet-wide listing is: what waits on the user in another repo
				// first, then this repo, then the rest — and flat when nothing waits elsewhere
				// (-> groupedLines). A note to the user IS what waits, which is what makes it foreign.
				local := localProject(st.Projects)
				listed := make([]listRow, 0, len(rows))
				for _, m := range rows {
					listed = append(listed, listRow{
						line:  mailLine(m),
						group: listGroupFor(m.Project, local, api.MailToUser(m) && !m.Read()),
					})
				}
				printListing(mailListTable, listed)
				fmt.Fprintln(os.Stderr, mailFooter(st, rows, f, agent))
				if note := limitNotice("message", len(rows), matched); note != "" {
					fmt.Fprint(os.Stderr, note)
				}
				return nil
			})
		},
	}
	c.Flags().StringVar(&agent, "agent", "", "only mail sent to this agent")
	c.Flags().BoolVar(&mine, "mine", false, "only mail addressed to you — what an agent has told you directly")
	c.Flags().StringVar(&filter, "filter", string(api.MailActive), "which mail to list: "+api.MailFilterNames())
	c.Flags().IntVar(&limit, "limit", DefaultListLimit, "show at most this many, newest first (0 = no limit)")
	return c
}

// mailListTable is the columns `sindri mail list` prints. Sender BEFORE recipient, the order mail is
// read in everywhere else: the two sat adjacent and unlabelled the other way round, and were misread
// over and over — labelling a backwards order would only have made the backwardness legible.
var mailListTable = table.Table{
	{Label: "id", Width: 8}, // wide enough for the rendered form (-> api.MailID), not the bare integer
	{Label: "repo", Width: 10, Clip: true},
	{Label: "from", Width: 10},
	{Label: "to", Width: 12},
	{Label: "state", Width: 14},
	{Label: "age", Width: 8},
	{Label: "message"},
}

// mailLine is one row: enough to tell whose it is, who sent it, whether it has been read, and what
// it says — with the body on one line, since the whole of it is what `mail show` is for.
func mailLine(m api.Mail) string {
	state := "unread"
	if m.Read() {
		state = "read"
	}
	if m.Pushed { // it was also injected live, so it may have been acted on already
		state += "+pushed"
	}
	return mailListTable.Line(
		table.Cell{Text: api.MailID(m.ID)},
		table.Cell{Text: m.Repo},
		table.Cell{Text: dash(m.Sender)},
		table.Cell{Text: m.Agent},
		table.Cell{Text: state},
		table.Cell{Text: shortAge(m.SentAt)},
		table.Cell{Text: oneLine(m.Body, 80)},
	)
}

// mailFooter says what the listing is NOT showing. The board carries a window of a mailbox that is
// never pruned, so a bare list of rows would quietly present the recent end as the whole history —
// and the point of keeping everything is being able to find last month's message.
func mailFooter(st api.BoardState, shown []api.Mail, f api.MailFilter, agent string) string {
	if st.MailTotal == 0 {
		return "no mail yet — a message an agent must read is kept here, and reading it only marks it read"
	}
	where := ""
	if agent != "" {
		where = " to " + agent
	}
	tail := ""
	if len(st.Mail) < st.MailTotal {
		tail = fmt.Sprintf(" Showing the last %d of %d messages; older mail is reachable by id "+
			"(`sindri mail show ml-<n>`).", len(st.Mail), st.MailTotal)
	}
	// The user's own unread is named separately, and fleet-wide: it is the number that asks something
	// of them, where the mailbox total merely says how much traffic there has been.
	mine := ""
	if st.MailUnreadUser > 0 {
		mine = fmt.Sprintf(" %d of them addressed to YOU (`sindri mail list --mine`).", st.MailUnreadUser)
	}
	return fmt.Sprintf("%d %s message(s)%s, %d unread across the fleet.%s%s",
		len(shown), f, where, st.MailUnread, mine, tail)
}

// mailReplyCmd answers an agent's message by its id. No recipient to type: it comes from the row, which
// is the point — the id is on every line of `mail list`.
func mailReplyCmd() *cobra.Command {
	return &cobra.Command{
		Use: "reply <id> <message...>", Short: "Answer a message an agent sent you (it goes to whoever sent it)",
		Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			id, err := api.ParseMailID(args[0])
			if err != nil {
				return err
			}
			msg := strings.Join(args[1:], " ")
			return withBackend(func(b backend) error {
				if err := b.ReplyToMail(id, msg); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "replied — it reads this at its next `sindri`, threaded with what you answered\n")
				return nil
			})
		},
	}
}

// mailShowState is the state word `mail show` prints — pulled out so it's testable without a
// backend. justRead names a mark this very call just made, which m.Read() cannot yet reflect.
func mailShowState(m api.Mail, justRead bool) string {
	switch {
	case justRead:
		return "read just now"
	case m.Read():
		return "read " + shortAge(m.ReadAt) + " ago"
	default:
		return "unread"
	}
}

func mailShowCmd() *cobra.Command {
	return &cobra.Command{
		Use: "show <id>", Short: "Show one message in full (the list carries only an opening)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id, err := api.ParseMailID(args[0])
			if err != nil {
				return err
			}
			return withBackend(func(b backend) error {
				m, err := b.MailBody(id)
				if err != nil {
					return err
				}
				// An explicit `mail show` is a deliberate read — the same act ENTER used to be — so it
				// marks at once rather than waiting on a dwell that has no cursor to time here.
				justRead := api.MailToUser(m) && !m.Read()
				if justRead {
					if err := b.MarkMailRead(id); err != nil {
						return err
					}
				}
				read := mailShowState(m, justRead)
				fmt.Printf("to:     %s (%s)\nfrom:   %s\nsent:   %s\nstate:  %s\npushed: %v\n\n%s\n",
					m.Agent, m.Repo, dash(m.Sender), m.SentAt, read, m.Pushed, strings.TrimRight(m.Body, "\n"))
				return nil
			})
		},
	}
}
