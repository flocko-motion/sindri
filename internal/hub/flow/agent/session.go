// package: hub/flow/agent / session
// type:    logic (the two things served ahead of any answer)
// job:     serve the mail an agent has not read, ahead of whatever its state then says.
// limits:  serving. What the agent is then told is its state's own text, and a clear a human armed
// is a state the machine stands it in (-> cond.ClearArmed).
package agent

import (
	"github.com/flo-at/sindri/internal/hub/flow/topic"
	"github.com/flo-at/sindri/internal/hub/prompts"
)

// ServeMail fetches an agent's unread mail, marks it read, and renders it as a directive preamble —
// "" when there is none.
func (a *Act) ServeMail(project, name string) (string, error) {
	ps := a.Store.For(project)
	msgs, err := ps.UnreadMail(name)
	if err != nil || len(msgs) == 0 {
		return "", err
	}
	for _, m := range msgs {
		if err := ps.MarkMailRead(m.ID); err != nil {
			return "", err
		}
	}
	a.Deps.Notify() // the unread count is on the board
	a.Flow.Wake(project, name, topic.MailArrived)
	return prompts.DirMail(msgs), nil
}
