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
			"agent is its entire purpose, and it is recorded per agent in `sindri agent info`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	c.AddCommand(mailListCmd(), mailShowCmd())
	return c
}

func mailListCmd() *cobra.Command {
	var agent, filter string
	c := &cobra.Command{
		Use: "list", Short: "List mail across the fleet, newest first", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			f, err := api.ParseMailFilter(filter)
			if err != nil {
				return err
			}
			return withBackend(func(b backend) error {
				st, err := b.State()
				if err != nil {
					return err
				}
				rows := api.FilterMail(f, agent, st.Mail)
				for _, m := range rows {
					fmt.Println(mailLine(m))
				}
				fmt.Fprintln(os.Stderr, mailFooter(st, rows, f, agent))
				return nil
			})
		},
	}
	c.Flags().StringVar(&agent, "agent", "", "only mail sent to this agent")
	c.Flags().StringVar(&filter, "filter", string(api.MailUnread), "which mail to list: "+api.MailFilterNames())
	return c
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
	return fmt.Sprintf("%-6d %-10.10s %-12s %-10s %-14s %-8s %s",
		m.ID, m.Repo, m.Agent, dash(m.Sender), state, shortAge(m.SentAt), oneLine(m.Body, 80))
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
			"(`sindri mail show <id>`).", len(st.Mail), st.MailTotal)
	}
	return fmt.Sprintf("%d %s message(s)%s, %d unread across the fleet.%s",
		len(shown), f, where, st.MailUnread, tail)
}

func mailShowCmd() *cobra.Command {
	return &cobra.Command{
		Use: "show <id>", Short: "Show one message in full (the list carries only an opening)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var id int64
			if _, err := fmt.Sscanf(args[0], "%d", &id); err != nil {
				return fmt.Errorf("mail id must be a number, got %q", args[0])
			}
			return withBackend(func(b backend) error {
				m, err := b.MailBody(id)
				if err != nil {
					return err
				}
				read := "unread"
				if m.Read() {
					read = "read " + shortAge(m.ReadAt) + " ago"
				}
				fmt.Printf("to:     %s (%s)\nfrom:   %s\nsent:   %s\nstate:  %s\npushed: %v\n\n%s\n",
					m.Agent, m.Repo, dash(m.Sender), m.SentAt, read, m.Pushed, strings.TrimRight(m.Body, "\n"))
				return nil
			})
		},
	}
}
