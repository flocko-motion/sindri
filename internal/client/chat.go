// package: client / chat
// type:    adapter (hub HTTP client)
// job:     the meeting room's half of the client: membership, saying something, the
// live stream, and the two verbs that end a meeting — new (clear the history)
// and close (empty the roster).
// limits:  transport only; what any of it MEANS is the hub's (-> hub/chat).
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/flo-at/sindri/internal/api"
)

// ChatAdd adds an agent to the user's chatroom (the hub greets it).
func (c *HTTP) ChatAdd(name string) error {
	return c.post("/chat/add", api.NameReq{Name: name})
}

// ChatRemove takes an agent out of the chatroom.
func (c *HTTP) ChatRemove(name string) error {
	return c.post("/chat/remove", api.NameReq{Name: name})
}

// ChatSay posts a message to the chatroom as the user (the discussion leader).
func (c *HTTP) ChatSay(msg string) error {
	return c.post("/chat/say", api.ChatSayReq{Msg: msg})
}

// CloseMeeting ends the meeting: every member is removed and told. The transcript stays, so a
// closed meeting can still be read (-> NewMeeting clears that).
func (c *HTTP) CloseMeeting() error { return c.post("/chat/close", struct{}{}) }

// NewMeeting clears the meeting's shared history and announces the fresh start to the room.
// Membership is untouched.
func (c *HTTP) NewMeeting() error {
	return c.post("/chat/new", struct{}{})
}

// ChatHeartbeat signals the user is present in the chatroom (sent periodically by
// `chat join` and the TUI chat tab). Presence keeps the room unlocked for agents.
func (c *HTTP) ChatHeartbeat() error {
	return c.post("/chat/heartbeat", struct{}{})
}

// Chat returns the current chatroom snapshot (members + recent transcript).
func (c *HTTP) Chat() (api.ChatView, error) {
	var v api.ChatView
	return v, c.get("/chat", &v)
}

// ChatWatch subscribes to the chatroom over SSE: it yields the snapshot on connect
// and a fresh one on every change, closing when ctx is cancelled or the hub goes
// away. This is the user's live leg of the star topology (the join CLI, TUI tab).
func (c *HTTP) ChatWatch(ctx context.Context) (<-chan api.ChatView, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/chat/stream", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	out := make(chan api.ChatView)
	go func() {
		defer resp.Body.Close()
		defer close(out)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var v api.ChatView
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &v) != nil {
				continue
			}
			select {
			case out <- v:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
