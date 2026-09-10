// package: hub/flow/fleet / directive
// type:    logic (the no-arg answer)
// job:     answer an agent asking where it stands: look once, read the state the machine settled
// on, and render it in that state's own words with the verbs it offers.
// limits:  reading and rendering. Deciding and moving are the machine's, on its own beat.
package fleet

import (
	"context"

	"github.com/flo-at/sindri/internal/hub/prompts"
	"github.com/flo-at/sindri/internal/hub/store"
)

// AgentDirective is the no-arg `sindri` answer, returned AT ONCE, mail served inline ahead of it
// (-> serveMail) unless a clear or a model switch lands this round instead.
func (e *Engine) AgentDirective(ctx context.Context, project, name string) (string, error) {
	// The backlog is re-synced for an agent that could claim from it right now. A sync reaches out to
	// every task source, and an ask by anybody else must not pay for that.
	if st, err := e.Store.For(project).GetState(name); err == nil && st.Task == "" && st.Container == "" {
		_ = e.taskAct().SyncTasks(project) // best-effort: the cached set answers if a source is unreachable
	}
	beforeModel := e.Harness.Observe(project, name).Model
	dir, err := e.directive(ctx, project, name)
	if err != nil {
		return "", err
	}
	// A model switch narrates and restarts the agent inline, with no distinct text to spot in dir —
	// only the model actually changing under this call says so.
	retiered := e.Harness.Observe(project, name).Model != beforeModel
	if prompts.Deferring(dir) || retiered {
		return dir, nil
	}
	preamble, err := e.roleAct().ServeMail(project, name)
	if err != nil {
		return "", err
	}
	// The branch warning goes ahead of the role text: an agent that cannot submit needs that before
	// it writes anything further, whatever its role would otherwise have said.
	return preamble + e.prAct().RebaseNotice(project, name) + dir, nil
}

// plannerDirective answers a planner from its phase alone — the backlog never enters it, since a
// planner's work arrives as a conversation. Shared with Kickoff, which serves this text directly.
func plannerDirective(st store.AgentState) string {
	switch st.Phase {
	case "submitted":
		return prompts.DirSubmitted
	case "planning": // set by AssignPlan and by `state planning` — it HAS work in hand
		return prompts.DirPlanning
	}
	return prompts.DirPlanner
}

// directive is the no-arg `sindri` answer: where the agent stands, in its own words, and what it may
// run there. It DECIDES nothing and it acts on nothing — the machine does both on its own beat, so
// asking is a read.
func (e *Engine) directive(_ context.Context, project, name string) (string, error) {
	// An ask IS an event, and the best moment to reconcile an agent is while it is there and
	// listening — so this looks first and then reports. Looking no longer changes the world as a
	// side effect of dispatch: the machine moves it, records the pass, and this reads the result.
	e.Flow.Look(project, name)
	s, err := e.flow.State(project + "/" + name)
	if err != nil {
		return "", err
	}
	w, err := e.gather(project, name)
	if err != nil {
		return "", err
	}
	words, err := e.speak(w, s)
	if err != nil {
		return "", err
	}
	return words + offerLines(s.Verbs), nil
}
