// package: hub/flow/roles / session
// type:    logic (the two things served ahead of any answer)
// job:     answer a request with what must reach the agent BEFORE its own words — a clear a human
// armed, and the mail it has not read.
// limits:  serving. What the agent is then told is its state's own text.
package roles

import (
	"context"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"

	"github.com/flo-at/sindri/internal/hub/prompts"
)

// FireClearIfArmed fires an armed clear now and wakes the agent once it lands. It reports whether it
// fired: the reply to THIS call would go into the session the clear just discarded.
func (a *Act) FireClearIfArmed(ctx context.Context, project, name string) (fired bool, err error) {
	if !a.ClearArmedFor(project, name) {
		return false, nil
	}
	if err := a.Harness.Clear(ctx, project, name); err != nil {
		return false, err
	}
	return true, a.Harness.Say(project, name, prompts.MsgKickoff, mail.PushOnly)
}

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
	return prompts.DirMail(msgs), nil
}
