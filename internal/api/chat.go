// package: api / chat
// type:    data (wire types)
// job:     the chatroom as it crosses the wire: its snapshot (roster + transcript)
// and the two row shapes behind it.
// limits:  data only; the relay, delivery and glyphs are the hub's / theme's.
package api

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
