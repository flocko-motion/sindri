// package: hub/workflow / retier
// type:    logic (matching a worker's model to its next task's tier)
// job:     change a free, idle worker onto the model its next task (or, holding a feature, its
// next subtask) needs — off the hub's tick, since the change compacts and restarts the
// worker, which must never land mid-command. Retired workers are exempt.
// limits:  the sweep; which task is next and whether it needs a change is retierDue's and
// containerRetierDue's (-> task.go), the actual change agent.Service.SetModel's.
package workflow

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/api"
)

// retierDue is claimNext's own pick, reduced to the fact its OTHER callers need: does the highest-
// priority claimable task want a different model than worker is currently running. Withheld the
// same way as a compaction due; the actual change is off-tick (-> FireDueRetiers).
func (e *Engine) retierDue(project, worker string) (tier string, due bool) {
	ps := e.store.For(project)
	packages, err := ps.OpenContainers()
	if err != nil {
		return "", false
	}
	leaves, err := ps.OpenLeaves()
	if err != nil {
		return "", false
	}
	t, _, ok := nextUp(packages, leaves, e.tierPrefers(project, worker))
	if !ok {
		return "", false
	}
	tier = api.TierOrDefault(t.Tier)
	want, known := e.deps.ModelForTier(tier)
	return tier, known && want != e.deps.CurrentModel(project, worker)
}

// containerRetierDue is retierDue's question asked one level down: a held feature's OWN next
// subtask, per sd-d17dcb ("one worker works the whole tree, changing model at each leaf boundary
// as the next subtask requires") — not the top-level pools claimNext reads.
func (e *Engine) containerRetierDue(project, agent, container string) (tier string, due bool) {
	children, err := e.store.For(project).OpenSubtasks(container)
	if err != nil || len(children) == 0 {
		return "", false
	}
	tier = api.TierOrDefault(children[0].Tier)
	want, known := e.deps.ModelForTier(tier)
	return tier, known && want != e.deps.CurrentModel(project, agent)
}

// FireDueRetiers is retierDue/containerRetierDue's decision, fired for every worker it applies to.
func (e *Engine) FireDueRetiers(project string) {
	ps := e.store.For(project)
	roster, err := ps.Roster()
	if err != nil {
		return
	}
	for _, a := range roster {
		if a.Retired || a.Role != "worker" || !e.deps.AgentAlive(project, a.Name) {
			continue
		}
		st, err := ps.GetState(a.Name)
		if err != nil || st.Task != "" {
			continue // mid-task: never here, whatever else is true
		}
		var tier string
		var due bool
		if st.Container != "" {
			tier, due = e.containerRetierDue(project, a.Name, st.Container)
		} else {
			tier, due = e.retierDue(project, a.Name)
		}
		if !due {
			continue
		}
		model, ok := e.deps.ModelForTier(tier)
		if !ok {
			continue
		}
		if err := e.deps.SetModel(project, a.Name, model); err != nil {
			fmt.Fprintf(os.Stderr, "hub: retiering %s to %s: %v\n", a.Name, model, err)
		}
	}
}
