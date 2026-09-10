// package: hub/flow/roles / assign_act
// type:    logic (the tiebreak between two equally-rated tasks)
// job:     answer, for one agent, whether a task's tier runs the model it already has — so a tie on
// priority is settled toward keeping the session it is in.
// limits:  the preference only; the ORDER it breaks a tie within is hub/task's (NextUp).
package roles

import (
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// TierPrefers builds nextUp's tiebreak for one agent: a task whose tier's model matches what it is
// currently running, so a tie on priority is resolved toward avoiding a model change. "" for agent
// (a hypothetical role, no one specific to avoid a change for) answers with no preference at all.
func (a *Act) TierPrefers(project, agent string) func(store.Task) bool {
	if agent == "" {
		return nil
	}
	current := a.Harness.Observe(project, agent).Model
	return func(t store.Task) bool {
		want, ok := a.Deps.ModelForTier(api.TierOrDefault(t.Tier))
		return ok && want == current
	}
}
