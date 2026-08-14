// package: ui/cli / chat
// type:    command (host CLI)
// job:     the user's control over the one meeting room: `meeting add`/`meeting remove`
// to curate who's in the room, `meeting join` to enter it interactively (read
// the live feed, type to broadcast as [user]), `meeting log` for the transcript,
// and bare `meeting` for who's in the room plus what you can do.
// limits:  thin calls into the backend; the relay + persistence live in the hub.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/theme"
	"github.com/spf13/cobra"
)

// NewChatCmd builds the `chat` command tree (the user's chatroom).
func NewChatCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "meeting",
		Short: "The user's meeting room: add/remove agents and join the discussion",
		Long: "The user's single meeting room — a star-topology relay: any member (an added " +
			"agent, or you) sends a message to the hub, which forwards it to everyone else. " +
			"Add agents to pull them in, then `meeting join` to lead the discussion; agents " +
			"talk with `sindri meeting <message>` from inside their pods.",
		Args: cobra.NoArgs,
		// Bare `sindri meeting`: who's in the room, then your options. The transcript moved to
		// `meeting log`, where asking for it is deliberate.
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = withBackend(func(b backend) error {
				v, err := b.Chat()
				if err != nil {
					return err
				}
				fmt.Print(renderMembers(v))
				if n := len(v.Log); n > 0 {
					fmt.Printf("%d recent message(s) — `sindri meeting log` to read them.\n", n)
				}
				return nil
			}) // a hub that isn't up shouldn't stop the help below from printing
			fmt.Println()
			return cmd.Help()
		},
	}
	c.AddCommand(chatAddCmd(), chatRemoveCmd(), chatJoinCmd(), chatLogCmd(), chatNewCmd(), chatCloseCmd())
	return c
}

// chatNewCmd starts a fresh meeting: the CLI half of the TUI's `N`, so the same operation is
// reachable from either front-end. No prompt, like every other destructive verb here — the CLI
// stays scriptable and the TUI is where a human gets the confirm modal.
func chatNewCmd() *cobra.Command {
	return &cobra.Command{
		Use: "new", Short: "Start a new meeting: clear the shared history (members stay)", Args: cobra.NoArgs,
		Long: "Clear the meeting's shared history and announce the fresh start to everyone in the " +
			"room. Membership is kept — this resets what the room remembers, not who is in it.\n\n" +
			"Agents are told the slate is clean, because they still hold the old discussion in their " +
			"own context. Anyone added afterwards is caught up with whatever has been said since.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withBackend(func(b backend) error {
				if err := b.NewMeeting(); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "new meeting — history cleared, members kept")
				return nil
			})
		},
	}
}

// chatCloseCmd ends the meeting: the CLI half of the TUI's `C`. No prompt, like every other
// destructive verb here — the CLI stays scriptable and the TUI is where a human gets the confirm.
func chatCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use: "close", Short: "Close the meeting: remove every member (the transcript is kept)", Args: cobra.NoArgs,
		Long: "End the meeting. Every member is removed and told, so nothing tries to speak into a " +
			"room that is over, and the roster stops being carried indefinitely.\n\n" +
			"The transcript is KEPT — a closed meeting can still be read (`sindri meeting log`); " +
			"`sindri meeting new` is what clears it. Re-adding members afterwards is manual, which " +
			"is why this is the deliberate end of a meeting rather than a pause.\n\n" +
			"A room with nothing said in it for an hour closes itself the same way.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withBackend(func(b backend) error {
				if err := b.CloseMeeting(); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "meeting closed — members removed, transcript kept "+
					"(`sindri meeting log` still reads it)")
				return nil
			})
		},
	}
}

// chatLogCmd prints the room transcript — what bare `meeting` used to do unasked.
func chatLogCmd() *cobra.Command {
	var n int
	c := &cobra.Command{
		Use: "log", Short: "Print the meeting transcript (members + recent messages)", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return withBackend(func(b backend) error {
				v, err := b.Chat()
				if err != nil {
					return err
				}
				if n > 0 && len(v.Log) > n { // keep the newest n
					v.Log = v.Log[len(v.Log)-n:]
				}
				fmt.Print(renderChat(v))
				return nil
			})
		},
	}
	c.Flags().IntVarP(&n, "tail", "n", 0, "print only the newest N messages (0 = the hub's whole window)")
	return c
}

func chatAddCmd() *cobra.Command {
	return &cobra.Command{
		Use: "add <agent...>", Short: "Add one or more agents to the meeting room", Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			for _, name := range args {
				// withAgent re-scopes the client to the agent's project, so a cross-repo agent is added under its own.
				if err := withAgent(name, func(b backend, a *api.AgentView) error {
					return b.ChatAdd(a.Name)
				}); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "added %s to the meeting room\n", name)
			}
			return nil
		},
	}
}

func chatRemoveCmd() *cobra.Command {
	c := &cobra.Command{
		Use: "remove <agent...>", Short: "Remove one or more agents from the meeting room", Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			for _, name := range args {
				if err := withAgent(name, func(b backend, a *api.AgentView) error {
					return b.ChatRemove(a.Name)
				}); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "removed %s from the meeting room\n", name)
			}
			return nil
		},
	}
	c.Aliases = []string{"rm"}
	return c
}

func chatJoinCmd() *cobra.Command {
	return &cobra.Command{
		Use: "join", Short: "Enter the meeting room interactively (read live, type to broadcast)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withBackend(func(b backend) error { return chatJoin(cmd, b) })
		},
	}
}

// chatJoin runs the interactive session: a background reader prints the hub's live stream while the
// foreground posts the user's lines as [user]. Ends on Ctrl-D or /quit.
func chatJoin(cmd *cobra.Command, b backend) error {
	if v, err := b.Chat(); err == nil {
		fmt.Print(renderMembers(v))
	}
	// Spell the interface out on entry — the same text the /help reply and the TUI use (theme.HelpText).
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Joined as %s %s — you lead the discussion; everything you type reaches every member.\n",
		theme.UserIcon, theme.NameStyle(theme.SenderUser).Render(theme.SenderUser))
	fmt.Fprintf(out, "  %s\n", theme.HelpLine())
	fmt.Fprintf(out, "  %s\n", theme.Dim().Render(
		"Ctrl-D or /quit to leave. Agents ("+theme.AgentIcon+") talk with `sindri meeting <message>` from their pods."))
	fmt.Fprintln(out, theme.Dim().Render("--- transcript ---"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Heartbeat while joined: the user is a required participant, so a live join keeps the room unlocked.
	_ = b.ChatHeartbeat()
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = b.ChatHeartbeat()
			}
		}
	}()
	ch, err := b.ChatWatch(ctx)
	if err != nil {
		return err
	}
	// The stream re-sends the whole transcript on every change; print only unseen ids. No locking:
	// this goroutine owns them.
	go func() {
		var lastSeen int64
		prev := "" // carried across stream pushes so grouping survives a reconnect
		for v := range ch {
			for _, m := range v.Log {
				if m.ID > lastSeen {
					fmt.Println(chatMsgLine(m, prev))
					lastSeen, prev = m.ID, m.Sender
				}
			}
		}
	}()

	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if line == "/quit" || line == "/q" {
			break
		}
		if err := b.ChatSay(line); err != nil {
			fmt.Fprintf(os.Stderr, "send failed: %v\n", err)
		}
	}
	return sc.Err()
}

// renderChat formats a chatroom snapshot: the member roster then the transcript.
func renderChat(v api.ChatView) string {
	var sb strings.Builder
	sb.WriteString(renderMembers(v))
	if len(v.Log) == 0 {
		sb.WriteString("(no messages yet)\n")
		return sb.String()
	}
	sb.WriteString(theme.Dim().Render("--- transcript ---") + "\n")
	prev := ""
	for _, m := range v.Log {
		sb.WriteString(chatMsgLine(m, prev) + "\n")
		prev = m.Sender
	}
	return sb.String()
}

// chatMsgLine formats one message as a speaker header (time · icon · coloured name) plus an indented
// body, so where one speaker stops and the next starts is obvious. prev is the previous sender: a new
// speaker gets a blank line and a header, a run from the same speaker prints body only.
func chatMsgLine(m api.ChatMessage, prev string) string {
	body := indentBody(m.Sender, m.Body)
	if m.Sender == prev {
		return body
	}
	head := theme.Icon(m.Sender) + " " + theme.NameStyle(m.Sender).Render(m.Sender)
	if hm := chatTS(m.TS); hm != "" {
		head = theme.Dim().Render(hm) + " " + head
	}
	lead := ""
	if prev != "" { // a blank line between speakers, but not above the first
		lead = "\n"
	}
	return lead + head + "\n" + body
}

// indentBody indents a body under its header in the speaker's colour. Styled per line, so each line
// carries its own escape codes and survives the terminal's soft-wrapping intact.
func indentBody(sender, body string) string {
	style := theme.BodyStyle(sender)
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for i, l := range lines {
		if l == "" {
			continue // a blank line needs no colour, and no escape codes around nothing
		}
		lines[i] = "  " + style.Render(l)
	}
	return strings.Join(lines, "\n")
}

// chatTS renders a stored RFC3339 timestamp as local HH:MM ("" if unparseable).
func chatTS(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}
	return t.Local().Format("15:04")
}

// renderMembers lists who is in the room, the user first: the roster stores only AGENTS, so an empty
// one means "no agents yet", not an empty room — the user is a permanent, required participant.
func renderMembers(v api.ChatView) string {
	parts := []string{theme.Icon(theme.SenderUser) + " " +
		theme.NameStyle(theme.SenderUser).Render(theme.SenderUser) + theme.Dim().Render(" (you)")}
	for _, m := range v.Members {
		entry := theme.Icon(m.Name) + " " + theme.NameStyle(m.Name).Render(m.Name)
		if m.Role != "" {
			entry += theme.Dim().Render(" (" + m.Role + ")")
		}
		parts = append(parts, entry)
	}
	line := "Meeting room — " + strings.Join(parts, " · ")
	if len(v.Members) == 0 {
		line += theme.Dim().Render(" — no agents yet; add one with `sindri meeting add <agent>`")
	}
	return line + "\n"
}
