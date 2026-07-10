// package: hub / chat
// type:    logic (chat wiring)
// job:     wire the extracted chat relay (internal/hub/chat) into the hub — provide
//          its Delivery port (tmux injection + board notify) and the thin methods
//          the server and registry call. The relay itself lives in the chat package.
// limits:  no chat logic here (-> internal/hub/chat); just the port + delegation.
package hub

import (
	"io"

	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/registry"
	"github.com/flo-at/sindri/internal/hub/store"
)

// chatDelivery adapts the hub to chat.Delivery: agent injection via tmux, a pod
// liveness check, and board notifications — the only hooks the chat service needs
// back into the hub.
type chatDelivery struct{ h *Hub }

func (c chatDelivery) Inject(project, name, text string) error {
	return c.h.inject(project, name, text)
}
func (c chatDelivery) Running(project, name string) bool {
	return container.Running(c.h.container(project, name))
}
func (c chatDelivery) Notify() { c.h.notify() }

// The hub keeps these thin methods so the server routes, registry, and board code
// call the chat relay without importing the chat package directly.

// ChatAdd puts an agent in the chatroom (and greets it).
func (h *Hub) ChatAdd(project, name string) error { return h.chat.Add(project, name) }

// ChatRemove takes an agent out of the chatroom.
func (h *Hub) ChatRemove(project, name string) error { return h.chat.Remove(project, name) }

// ChatMembers returns the current room roster.
func (h *Hub) ChatMembers() ([]store.ChatMember, error) { return h.chat.Members() }

// ChatTranscript returns the recent room transcript (oldest first).
func (h *Hub) ChatTranscript(limit int) ([]store.ChatMessage, error) {
	return h.chat.Transcript(limit)
}

// ChatSay forwards a message from the user to the room.
func (h *Hub) ChatSay(msg string) (store.ChatMessage, error) { return h.chat.Say(msg) }

// ChatUserMessage handles one line the user submits (a slash command or a message).
func (h *Hub) ChatUserMessage(line string) error { return h.chat.UserMessage(line) }

// ChatHeartbeat records the user's presence in the chatroom.
func (h *Hub) ChatHeartbeat() { h.chat.Heartbeat() }

// cmdChat is the agent-facing `chat` verb (registered in commands.go).
func (h *Hub) cmdChat(c registry.Caller, args []string, out io.Writer) (int, error) {
	return h.chat.Cmd(c, args, out)
}
