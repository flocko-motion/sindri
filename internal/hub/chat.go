// package: hub / chat
// type:    logic (chat wiring)
// job:     wire the extracted chat relay (internal/hub/chat) into the hub — provide
//
//	its Delivery port (tmux injection + board notify) and the agent-facing
//	`chat` verb. Everything else calls h.chat directly; the relay itself
//	lives in the chat package.
//
// limits:  no chat logic here (-> internal/hub/chat); just the port + the verb.
package hub

import (
	"io"

	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/registry"
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

// cmdChat is the agent-facing `chat` verb (registered in commands.go).
func (h *Hub) cmdChat(c registry.Caller, args []string, out io.Writer) (int, error) {
	return h.chat.Cmd(c, args, out)
}
