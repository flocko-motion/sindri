// package: hub/flow/agent / assign
// type:    logic (the tiebreak between two equally-rated tasks)
// job:     answer, for one agent, whether a task's tier runs the model it already has — so a tie on
// priority is settled toward keeping the session it is in.
// limits:  the preference only; the ORDER it breaks a tie within is hub/task's (NextUp).
package agent

import (
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// TierPrefers builds nextUp's tiebreak for one agent: a task whose tier dispatches to the model it
// is already running, so a tie on priority resolves toward avoiding a switch. It takes that reading
// rather than fetching one (-> Situation.Model), since the answer cannot change inside a sort and
// the caller already holds it. "" for on (a hypothetical role, nobody's session to spare) answers
// with no preference at all.
func (a *Act) TierPrefers(on string) func(store.Task) bool {
	if on == "" {
		return nil
	}
	return func(t store.Task) bool {
		met, known := a.Harness.TierIs(on, api.TierOrDefault(t.Tier))
		return known && met
	}
}
