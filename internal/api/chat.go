// package: api / chat
// type:    logic (wire types + pure predicates)
// job:     the chatroom as it crosses the wire: its snapshot (roster + transcript), the two row
// shapes behind it, and the sender vocabulary those rows carry.
// limits:  data and pure derivation only — glyphs and help text are a front-end's call
// (-> internal/ui/theme); the relay lives in hub/chat.
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
	// SenderUser is the human, in both directions: the sender of a message they type, and the RECIPIENT
	// of an agent's note (-> Mail.Agent). One spelling, so no agent may be named it.
	SenderUser   = "user"
	SenderSystem = "system" // the hub speaking for itself
)

// IsChatCommand reports whether a line is an in-room slash command rather than a message —
// the hub dispatches on it, and a front-end's composer needs the identical rule to decide
// whether enter submits (a command) or inserts a newline (a message can be multiline).
func IsChatCommand(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "/")
}
