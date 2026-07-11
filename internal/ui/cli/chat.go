// package: ui/cli / chat
// type:    command (host CLI)
// job:     the user's control over the one chatroom: `chat add`/`chat remove` to
//          curate who's in the room, `chat join` to enter it interactively (read
//          the live feed, type to broadcast as [user]), and bare `chat` for a
//          one-shot snapshot of members + transcript.
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
	"github.com/spf13/cobra"
)

// NewChatCmd builds the `chat` command tree (the user's chatroom).
func NewChatCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "chat",
		Short: "The user's chatroom: add/remove agents and join the discussion",
		Long: "The user's single chatroom — a star-topology relay: any member (an added " +
			"agent, or you) sends a message to the hub, which forwards it to everyone else. " +
			"Add agents to pull them in, then `chat join` to lead the discussion; agents " +
			"talk with `sindri chat <message>` from inside their pods.",
		Args: cobra.NoArgs,
		// Bare `sindri chat`: a one-shot snapshot (members + recent transcript).
		RunE: func(_ *cobra.Command, _ []string) error {
			return withBackend(func(b backend) error {
				v, err := b.Chat()
				if err != nil {
					return err
				}
				fmt.Print(renderChat(v))
				return nil
			})
		},
	}
	c.AddCommand(chatAddCmd(), chatRemoveCmd(), chatJoinCmd())
	return c
}

func chatAddCmd() *cobra.Command {
	return &cobra.Command{
		Use: "add <agent...>", Short: "Add one or more agents to the chatroom", Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			for _, name := range args {
				// withAgent resolves the agent's project (re-scoping the client), so an
				// agent in another repo is added under its own project.
				if err := withAgent(name, func(b backend, a *hub.AgentView) error {
					return b.ChatAdd(a.Name)
				}); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "added %s to the chatroom\n", name)
			}
			return nil
		},
	}
}

func chatRemoveCmd() *cobra.Command {
	c := &cobra.Command{
		Use: "remove <agent...>", Short: "Remove one or more agents from the chatroom", Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			for _, name := range args {
				if err := withAgent(name, func(b backend, a *hub.AgentView) error {
					return b.ChatRemove(a.Name)
				}); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "removed %s from the chatroom\n", name)
			}
			return nil
		},
	}
	c.Aliases = []string{"rm"}
	return c
}

func chatJoinCmd() *cobra.Command {
	return &cobra.Command{
		Use: "join", Short: "Enter the chatroom interactively (read live, type to broadcast)", Args: cobra.NoArgs,
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
	fmt.Fprintln(cmd.OutOrStdout(), "Joined the chatroom. Type a message + Enter to send to everyone; /help for commands (/add, /remove, /who); Ctrl-D or /quit to leave.")
	fmt.Fprintln(cmd.OutOrStdout(), "--- transcript ---")

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
		for v := range ch {
			for _, m := range v.Log {
				if m.ID > lastSeen {
					fmt.Println(chatMsgLine(m))
					lastSeen = m.ID
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
	sb.WriteString("--- transcript ---\n")
	for _, m := range v.Log {
		sb.WriteString(chatMsgLine(m) + "\n")
	}
	return sb.String()
}

// chatMsgLine formats a transcript message as "HH:MM sender: body" (local time;
// the timestamp is dropped if unparseable). The terminal soft-wraps long lines.
func chatMsgLine(m store.ChatMessage) string {
	if t, err := time.Parse(time.RFC3339, m.TS); err == nil {
		return fmt.Sprintf("%s %s: %s", t.Local().Format("15:04"), m.Sender, m.Body)
	}
	return fmt.Sprintf("%s: %s", m.Sender, m.Body)
}

// renderMembers formats just the member line (name + role), or a note when empty.
func renderMembers(v hub.ChatView) string {
	if len(v.Members) == 0 {
		return "Chatroom is empty — add agents with `sindri chat add <agent>`.\n"
	}
	names := make([]string, len(v.Members))
	for i, m := range v.Members {
		if m.Role != "" {
			names[i] = fmt.Sprintf("%s (%s)", m.Name, m.Role)
		} else {
			names[i] = m.Name
		}
	}
	return fmt.Sprintf("Chatroom — %d member(s): %s\n", len(v.Members), strings.Join(names, ", "))
}
