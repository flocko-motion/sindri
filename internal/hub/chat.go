// package: hub / chat
// type:    logic (the chatroom relay — star topology)
// job:     star-centre relay for the user's one chatroom: hold membership +
//          transcript (-> store/chat.go) and forward every message to all
//          participants — agents via tmux injection, the user's live views via
//          the board notify. add/remove push a greeting.
// limits:  persistence is in store/chat.go; HTTP wiring in server.go; the agent's
//          `chat` verb in commands.go. No UI here.
package hub

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// chatUser is the sender label for messages the human types (via `sindri chat
// join` or the TUI tab); chatSystem labels the hub's own lines (join/leave
// announcements, command replies). Both are distinct from any agent name.
const (
	chatUser   = "user"
	chatSystem = "system"
)

// chatTranscriptLimit bounds how much history a snapshot / live view carries.
const chatTranscriptLimit = 200

// chatPresenceTTL is how long a chatroom stays unlocked after the user's last
// heartbeat. The user is a required participant: their `chat join` / TUI chat tab
// beats every few seconds; once the beats stop for this long the room locks and
// agents can't post — the discussion is the user's, so it freezes when they leave.
const chatPresenceTTL = 20 * time.Second

// chatMaxLen caps a single chat message — generous enough for deep, multi-paragraph
// discussion, but bounded so one message can't be a novel (and a huge tmux inject).
// Enforced in chatBroadcast, so every path (agent, CLI, TUI) gets the same limit
// and the same feedback rather than a silent truncation.
const chatMaxLen = 4000

// errChatTooLong is returned when a message exceeds chatMaxLen. It's an actionable
// message meant for whoever sent it (not a hub-internal error), so callers surface
// it to the sender.
var errChatTooLong = fmt.Errorf("message too long — keep it under %d characters (split a longer one into parts)", chatMaxLen)

// chatHelpText lists the in-chat slash commands (IRC-style) the user can type into
// the room — the hub executes them instead of broadcasting them.
const chatHelpText = "commands: /add <agent> (alias /invite) · /remove <agent> (alias /kick) · /who (list members) · /help. Anything not starting with / is sent to everyone in the room."

// ChatAdd puts an agent in the chatroom and greets it (so it knows it's in and how
// to speak). Errors if the agent doesn't exist or is already a member — both are
// the operator's to see. The greeting is best-effort: a stopped agent misses the
// live line but is still a member, and gets reminded on relaunch (see rehydrate).
func (h *Hub) ChatAdd(project, name string) error {
	if _, ok, err := h.store.For(project).GetAgent(name); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	member, err := h.store.ChatIsMember(project, name)
	if err != nil {
		return err
	}
	if member {
		return fmt.Errorf("%s is already in the chatroom", name)
	}
	if err := h.store.ChatAdd(project, name); err != nil {
		return err
	}
	h.chatDeliver(project, name, msgChatWelcome)
	// Announce to the room (IRC-style), so everyone — agents and the user's views —
	// sees who joined. chatBroadcast records + forwards + notifies.
	_, err = h.chatBroadcast("", chatSystem, name+" joined the chatroom")
	return err
}

// ChatRemove takes an agent out of the chatroom and tells it so. Errors if it
// wasn't a member (nothing to remove).
func (h *Hub) ChatRemove(project, name string) error {
	was, err := h.store.ChatRemove(project, name)
	if err != nil {
		return err
	}
	if !was {
		return fmt.Errorf("%s is not in the chatroom", name)
	}
	h.chatDeliver(project, name, msgChatRemoved)
	// Announce the departure to whoever's left (the removed agent is no longer a
	// member, so it won't receive this).
	_, err = h.chatBroadcast("", chatSystem, name+" left the chatroom")
	return err
}

// ChatHeartbeat records that the user is present in the chatroom (called
// periodically by `chat join` and the TUI chat tab while they're open). Presence
// keeps the room unlocked; see chatPresent.
func (h *Hub) ChatHeartbeat() {
	h.chatMu.Lock()
	h.chatSeen = time.Now()
	h.chatMu.Unlock()
}

// chatPresent reports whether the user is currently in the room — a heartbeat
// within chatPresenceTTL. When false the room is locked and agents can't post.
func (h *Hub) chatPresent() bool {
	h.chatMu.Lock()
	seen := h.chatSeen
	h.chatMu.Unlock()
	return !seen.IsZero() && time.Since(seen) < chatPresenceTTL
}

// ChatMembers returns the current room roster.
func (h *Hub) ChatMembers() ([]store.ChatMember, error) {
	return h.store.ChatMembers()
}

// ChatTranscript returns the recent room transcript (oldest first). A non-positive
// limit falls back to the default window.
func (h *Hub) ChatTranscript(limit int) ([]store.ChatMessage, error) {
	if limit <= 0 {
		limit = chatTranscriptLimit
	}
	return h.store.ChatTranscript(limit)
}

// ChatSay forwards a message from the user (the discussion leader) to the room.
func (h *Hub) ChatSay(msg string) (store.ChatMessage, error) {
	return h.chatBroadcast("", chatUser, msg)
}

// ChatUserMessage handles one line the user submits to the room. A line starting
// with "/" is an IRC-style command the hub executes (add/remove/who/help — the
// user's membership controls); anything else is broadcast as a normal [user]
// message. Only this (the user's) path interprets commands, so agents — which
// speak via cmdChat — can never change membership. Both `sindri chat join` and the
// TUI tab post here, so they share the command set.
func (h *Hub) ChatUserMessage(line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return fmt.Errorf("empty message")
	}
	h.ChatHeartbeat() // the user just posted — they're present, keep the room unlocked
	if strings.HasPrefix(line, "/") {
		return h.chatCommand(line)
	}
	_, err := h.ChatSay(line)
	return err
}

// chatCommand parses and runs a "/command". Its result (or error) goes back as a
// system line in the transcript, visible to the user's views but not injected into
// agents' sessions — command feedback is the user's, not the room's.
func (h *Hub) chatCommand(line string) error {
	fields := strings.Fields(line)
	verb := strings.ToLower(strings.TrimPrefix(fields[0], "/"))
	args := fields[1:]
	switch verb {
	case "add", "invite":
		return h.chatMembershipCmd(args, true)
	case "remove", "rm", "kick":
		return h.chatMembershipCmd(args, false)
	case "who", "names", "members":
		return h.chatSystemReply(h.chatWhoLine())
	case "help", "?":
		return h.chatSystemReply(chatHelpText)
	case "quit", "q", "leave", "part", "exit":
		// Recognized so it's never an "unknown command": `chat join` intercepts these
		// locally to end its loop (a REPL has to); the TUI has no session to quit, so
		// it lands here with guidance instead.
		return h.chatSystemReply("to leave: press Ctrl-D or type /quit in `sindri chat join`; in the TUI, just switch tabs.")
	default:
		return h.chatSystemReply("unknown command /" + verb + " — type /help for the list")
	}
}

// chatMembershipCmd runs /add or /remove for one or more agents, resolving each by
// name across all projects (names are globally unique, so the join session's own
// project doesn't constrain which agents can be added). ChatAdd/ChatRemove announce
// the join/leave; only failures (typo'd name, already/not a member) get a reply.
func (h *Hub) chatMembershipCmd(args []string, add bool) error {
	if len(args) == 0 {
		if add {
			return h.chatSystemReply("usage: /add <agent>… (alias /invite)")
		}
		return h.chatSystemReply("usage: /remove <agent>… (alias /kick)")
	}
	for _, name := range args {
		project, ok, err := h.findAgentProject(name)
		if err != nil {
			return err
		}
		if !ok {
			if rerr := h.chatSystemReply(fmt.Sprintf("no agent named %q", name)); rerr != nil {
				return rerr
			}
			continue
		}
		var aerr error
		if add {
			aerr = h.ChatAdd(project, name)
		} else {
			aerr = h.ChatRemove(project, name)
		}
		if aerr != nil { // e.g. "already in the chatroom" / "not in the chatroom"
			if rerr := h.chatSystemReply(aerr.Error()); rerr != nil {
				return rerr
			}
		}
	}
	return nil
}

// chatWhoLine renders the /who reply: the current roster (name + role), or a nudge
// when the room is empty.
func (h *Hub) chatWhoLine() string {
	members, err := h.store.ChatMembers()
	if err != nil {
		return "couldn't read the member list"
	}
	if len(members) == 0 {
		return "the chatroom is empty — /add <agent> to bring someone in"
	}
	parts := make([]string, len(members))
	for i, m := range members {
		if m.Role != "" {
			parts[i] = fmt.Sprintf("%s (%s)", m.Name, m.Role)
		} else {
			parts[i] = m.Name
		}
	}
	return fmt.Sprintf("members (%d): %s", len(members), strings.Join(parts, ", "))
}

// chatSystemReply records a hub reply in the transcript (sender "system") and wakes
// the user's live views. It is NOT injected into agents' sessions — it's feedback
// for the user who typed the command, not chatter for the room.
func (h *Hub) chatSystemReply(text string) error {
	if _, err := h.store.ChatAppend(chatSystem, text); err != nil {
		return err
	}
	h.notify()
	return nil
}

// findAgentProject resolves an agent's project by its (globally unique) name.
func (h *Hub) findAgentProject(name string) (string, bool, error) {
	agents, err := h.store.AllAgents()
	if err != nil {
		return "", false, err
	}
	for _, a := range agents {
		if a.Name == name {
			return a.Project, true, nil
		}
	}
	return "", false, nil
}

// cmdChat is the agent-facing `chat` verb: forward the agent's message to the
// room. Only reachable by members (the registry hides it otherwise), so no
// membership re-check is needed here.
func (h *Hub) cmdChat(c registry.Caller, args []string, out io.Writer) (int, error) {
	msg := strings.TrimSpace(strings.Join(args, " "))
	if msg == "" {
		fmt.Fprintln(out, "usage: chat <message...>")
		return 2, nil
	}
	// The user is a required participant: with no one present, the room is locked and
	// the message goes nowhere. Tell the agent to hold until the user is back.
	if !h.chatPresent() {
		fmt.Fprintln(out, "the chatroom is locked — the user has stepped away, so your message wasn't sent. Wait, then send it again once they're back.")
		return 1, nil
	}
	if _, err := h.chatBroadcast(c.Project, c.Agent, msg); err != nil {
		// "too long" is the agent's to act on (trim + retry), so show it directly —
		// AgentExec would otherwise neutralize a returned error into a generic notice.
		if errors.Is(err, errChatTooLong) {
			fmt.Fprintln(out, err.Error())
			return 1, nil
		}
		return 1, err
	}
	fmt.Fprintln(out, "sent to the chatroom")
	return 0, nil
}

// chatBroadcast is the star centre: record the message in the transcript, then
// forward it to every member agent (skipping the sender, who already has its own
// words), and wake the user's live views via notify.
func (h *Hub) chatBroadcast(senderProject, senderName, body string) (store.ChatMessage, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return store.ChatMessage{}, fmt.Errorf("empty message")
	}
	if utf8.RuneCountInString(body) > chatMaxLen {
		return store.ChatMessage{}, errChatTooLong
	}
	msg, err := h.store.ChatAppend(senderName, body)
	if err != nil {
		return store.ChatMessage{}, err
	}
	members, err := h.store.ChatMembers()
	if err != nil {
		return store.ChatMessage{}, err
	}
	line := fmt.Sprintf("[chat] %s: %s", senderName, body)
	for _, m := range members {
		if m.Project == senderProject && m.Name == senderName {
			continue // the sender already typed this in its own terminal
		}
		h.chatDeliver(m.Project, m.Name, line)
	}
	h.notify()
	return msg, nil
}

// chatDeliver injects one chat line into an agent's session and records it on that
// agent's activity log. Best-effort: an offline agent is skipped (it reads the room
// when it's back), and an injection error is logged host-side (the operator's
// channel) — a chat line is never worth failing a broadcast or an add. The message
// is in the transcript regardless, so nothing is truly lost.
func (h *Hub) chatDeliver(project, name, line string) {
	if !container.Running(h.container(project, name)) {
		return
	}
	if err := h.inject(project, name, line); err != nil {
		fmt.Fprintf(os.Stderr, "hub: chat delivery to %s/%s failed: %v\n", project, name, err)
		return
	}
	if err := h.store.For(project).Log(name, "chat", line); err != nil {
		fmt.Fprintf(os.Stderr, "hub: logging chat for %s/%s failed: %v\n", project, name, err)
	}
}
