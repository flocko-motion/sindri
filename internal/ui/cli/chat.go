// package: ui/cli / chat
// type:    command (host CLI)
// job:     the user's control over the one meeting room: `meeting add`/`meeting remove`
//
//	to curate who's in the room, `meeting join` to enter it interactively (read
//	the live feed, type to broadcast as [user]), `meeting log` for the transcript,
//	and bare `meeting` for who's in the room plus what you can do.
//
// limits:  thin calls into the backend; the relay + persistence live in the hub.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
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
		// Bare `sindri meeting`: who's in the room, then what you can do. It used to dump
		// the whole transcript (up to 200 messages), which buried the one thing a bare
		// command should answer — what are my options. The transcript moved to `meeting log`,
		// where asking for it is deliberate.
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
	c.AddCommand(chatAddCmd(), chatRemoveCmd(), chatJoinCmd(), chatLogCmd())
	return c
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
				// withAgent resolves the agent's project (re-scoping the client), so an
				// agent in another repo is added under its own project.
				if err := withAgent(name, func(b backend, a *hub.AgentView) error {
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
				if err := withAgent(name, func(b backend, a *hub.AgentView) error {
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

// chatJoin runs the interactive session: a background reader prints every new
// message from the hub's live stream, while the foreground reads the user's lines
// and posts them as [user]. Ends on Ctrl-D (EOF) or /quit.
func chatJoin(cmd *cobra.Command, b backend) error {
	if v, err := b.Chat(); err == nil {
		fmt.Print(renderMembers(v))
	}
	// Spell the interface out on entry. The one-liner this replaces mentioned /help and
	// left it at that, so the commands that actually matter — pulling agents INTO the room
	// — were a guess away. Same text the /help reply and the TUI use (hub.ChatHelpText).
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Joined as %s %s — you lead the discussion; everything you type reaches every member.\n",
		theme.UserIcon, theme.NameStyle(theme.SenderUser).Render(theme.SenderUser))
	fmt.Fprintf(out, "  %s\n", theme.HelpLine())
	fmt.Fprintf(out, "  %s\n", theme.Dim().Render(
		"Ctrl-D or /quit to leave. Agents ("+theme.AgentIcon+") talk with `sindri meeting <message>` from their pods."))
	fmt.Fprintln(out, theme.Dim().Render("--- transcript ---"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Heartbeat while joined: the user is a required participant, so a live join keeps
	// the room unlocked. Beat now and every few seconds until the session ends.
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
	// The stream re-sends the whole recent transcript on connect and on every
	// change; print only messages we haven't shown yet (monotonic ids). Owned
	// solely by this goroutine, so no locking.
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
func renderChat(v hub.ChatView) string {
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

// chatMsgLine formats one transcript message as a speaker header (time · icon · name,
// the name in its own deterministic colour) followed by the indented body. Every line
// of a multi-line message keeps that indent, so where one person stops and the next
// starts is obvious at a glance — the flat "HH:MM sender: body" it replaces ran
// everyone's words together into an unreadable wall.
//
// prev is the previous message's sender ("" for the first): a change of speaker gets a
// blank line before the header, and a run from the same speaker prints body only. That
// grouping is what makes a long transcript skimmable.
func chatMsgLine(m store.ChatMessage, prev string) string {
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

// indentBody indents every line of a message body under its header and paints it in the
// speaker's colour. Styled per line rather than as one block, so each line carries its own
// escape codes and survives the terminal's own soft-wrapping intact.
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

// renderMembers lists who is in the room, the user first. The stored roster holds only
// AGENTS — the user is a permanent participant (and a required one: presence unlocks the
// room), never a row in it. So an empty roster meant "no agents yet", and calling that
// "the meeting room is empty" told the user they weren't somewhere they were standing.
// Listing them explicitly also shows which name and colour agents will see them under.
func renderMembers(v hub.ChatView) string {
	parts := []string{theme.Icon(theme.SenderUser) + " " +
		theme.NameStyle(theme.SenderUser).Render(theme.SenderUser) + theme.Dim().Render(" (you)")}
	for _, m := range v.Members {
		entry := theme.Icon(m.Name) + " " + theme.NameStyle(m.Name).Render(m.Name)
		if m.Role != "" {
			entry += theme.Dim().Render(" ("+m.Role+")")
		}
		parts = append(parts, entry)
	}
	line := "Meeting room — " + strings.Join(parts, " · ")
	if len(v.Members) == 0 {
		line += theme.Dim().Render(" — no agents yet; add one with `sindri meeting add <agent>`")
	}
	return line + "\n"
}
