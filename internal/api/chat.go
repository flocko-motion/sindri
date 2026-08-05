// package: api / chat
// type:    data (wire types + pure predicates)
// job:     the chatroom as it crosses the wire: its snapshot (roster + transcript),
// the two row shapes behind it, and the Sender vocabulary + glyphs — protocol,
// not decoration: the hub stamps them into the line an agent reads, and every
// front-end renders the identical marker for the identical sender.
// limits:  data and pure derivation only; the relay lives in hub/chat.
package api

import "strings"

// ChatMember is one agent in the chatroom. Role is filled from the agents table
// (empty if the agent no longer exists — a stale membership pending cleanup).
type ChatMember struct {
	Project string `json:"project"`
	Name    string `json:"name"`
	Role    string `json:"role"`
}

// ChatMessage is one line of the room transcript. Sender is an agent name or
// "user" (the human leading the discussion). ID is monotonic, so a live viewer can
// print only messages newer than the last it saw.
type ChatMessage struct {
	ID     int64  `json:"id"`
	Sender string `json:"sender"`
	Body   string `json:"body"`
	TS     string `json:"ts"`
}

// ChatView is the chatroom snapshot served by GET /chat and streamed by
// GET /chat/stream: the current member roster and the recent transcript.
type ChatView struct {
	Members []ChatMember  `json:"members"`
	Log     []ChatMessage `json:"log"`
}

// Sender values a ChatMessage carries beyond an agent's own name.
const (
	SenderUser   = "user"   // the human — the one participant an agent must never mistake for a peer
	SenderSystem = "system" // the hub speaking for itself
)

// Chat participant glyphs, plain UTF-8 (ANSI would land as noise once stamped into
// the line an agent reads).
const (
	UserIcon   = "👤"
	AgentIcon  = "🤖"
	SystemIcon = "⚙"
)

// ChatIcon marks who is speaking: the human, the hub itself, or an agent.
func ChatIcon(sender string) string {
	switch sender {
	case SenderUser:
		return UserIcon
	case SenderSystem:
		return SystemIcon
	}
	return AgentIcon
}

// ChatHelpText is exported so `/help`, the `meeting join` banner and the TUI chat tab can't
// drift into three different accounts of the same commands.
const ChatHelpText = "/add <agent> (alias /invite) · /remove <agent> (alias /kick) · /who (list members) · /help. " +
	"Anything not starting with / is sent to everyone in the room."

// IsChatCommand reports whether a line is an in-room slash command rather than a message —
// the hub dispatches on it, and a front-end's composer needs the identical rule to decide
// whether enter submits (a command) or inserts a newline (a message can be multiline).
func IsChatCommand(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "/")
}
