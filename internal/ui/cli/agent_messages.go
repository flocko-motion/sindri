// package: ui/cli / agent messages
// type:    command (host CLI)
// job:     the two ways a user reaches an agent, as two actions: `tell` PUSHES into the live
// session now, and `mail` WAITS to be read without interrupting. Each one-liner says
// which it is, because that is where the choice is made.
// limits:  thin calls into the backend; the delivery model is the hub's (-> workflow.Delivery).
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/spf13/cobra"
)

// agentMailCmd is the other way to reach an agent: it WAITS to be read instead of interrupting, and
// so reaches one that `tell` cannot — down, restarting, mid-clear or signed out. Deliberately no push:
// choosing mail IS the choice not to interrupt, and notifying anyway would defeat picking it.
func agentMailCmd() *cobra.Command {
	return &cobra.Command{
		Use: "mail <name> <message...>", Short: "Mail an agent: it waits to be read, and never interrupts ([user])",
		Long: "Put a message in an agent's mailbox, stamped [user].\n\n" +
			"Mail WAITS: the agent reads it when it next asks the hub what to do, and it is never lost —\n" +
			"so this is the verb for an agent that is down, restarting or signed out, and for anything of\n" +
			"the \"when you get to it, note that X\" kind.\n\n" +
			"`sindri agent tell` is the other choice: it types into the live session NOW, which interrupts\n" +
			"and is lost if the agent is not there to receive it. Use tell to stop an agent, mail to inform\n" +
			"one.",
		Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			msg := strings.Join(args[1:], " ")
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				if err := b.MailAgent(a.Name, msg); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "mailed %s — it reads this at its next `sindri`, whatever it is doing now\n", a.Name)
				return nil
			})
		},
	}
}

func agentTellCmd() *cobra.Command {
	var restart, anyway bool
	c := &cobra.Command{
		Use: "tell <name> <message...>", Short: "Push a message into an agent's live session now — interrupts, and lost if it is away ([user])", Args: cobra.MinimumNArgs(2),
		Long: "Send a message into an agent's session, stamped [user].\n\n" +
			"An agent whose pane reads signed out is asked about rather than written to: nothing typed at a\n" +
			"/login prompt is sent, so the message would sit in its input box unread. Both answers are yours\n" +
			"to give — --restart bounces it first, which is how it re-reads the credentials the hub keeps\n" +
			"staged, and --anyway sends regardless, for when you have just renewed the host's token and know\n" +
			"better than the pane does.",
		RunE: func(_ *cobra.Command, args []string) error {
			msg := strings.Join(args[1:], " ")
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				signedOut, err := tellSignedOut(a, restart, anyway)
				if err != nil {
					return err
				}
				if signedOut == api.SignedOutRestart {
					fmt.Fprintf(os.Stderr, "restarting %s, then sending — the session resumes…\n", a.Name)
				}
				if err := b.Tell(a.Name, msg, "user", signedOut); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "delivered to %s\n", a.Name)
				return nil
			})
		},
	}
	c.Flags().BoolVar(&restart, "restart", false, "if it reads signed out, restart it first (it re-reads the host's credentials), then send")
	c.Flags().BoolVar(&anyway, "anyway", false, "send even if it reads signed out — the pane's reading may be out of date")
	return c
}

// tellSignedOut is the answer the message carries about a signed-out pane: the one the user gave,
// or a refusal that names both remedies rather than describing one it cannot perform. The check is
// on the board's word, so the refusal only greets an agent that actually reads signed out.
func tellSignedOut(a *api.AgentView, restart, anyway bool) (string, error) {
	switch {
	case restart && anyway:
		return "", fmt.Errorf("--restart and --anyway ask for different things — pick one")
	case restart:
		return api.SignedOutRestart, nil
	case anyway:
		return api.SignedOutSend, nil
	case a.Status == api.StatusSignedOut:
		return "", fmt.Errorf("%s reads signed out — nothing typed at a /login prompt is sent, so the message "+
			"would sit unread in its input box. Restart it and send, which makes the process re-read the "+
			"credentials the hub keeps staged: `sindri agent tell %s --restart <message>`. Or send regardless, "+
			"if you know the session is fine: `--anyway`. (If the host is signed out too, log in there first.)",
			a.Name, a.Name)
	}
	return api.SignedOutRefuse, nil
}
