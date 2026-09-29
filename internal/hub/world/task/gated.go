// package: hub/world/task / gated
// type:    logic (what still holds a tree open)
// job:     answer which children under a container are not yet finished, and name them — the
// question every "can this go out" asks, in one place.
// limits:  the reading. What a held-open tree MEANS to an agent is the role's map.
package task

import (
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// GatedUnder is open work under a feature still AWAITING A VERDICT, at any depth — the COMPLETION
// question, since such a subtask is ABSENT from OpenSubtasks rather than reported by it. Pending
// only, never the claim rule: nothing clears a rejection, so blocking on one parks the holder.
func GatedUnder(ps *store.ProjectStore, container string) ([]store.Task, error) {
	all, err := ps.AllTasks()
	if err != nil {
		return nil, err
	}
	var out []store.Task
	for _, d := range api.Descendants(all, container) {
		if api.Open(d) && d.Approval == "pending" {
			out = append(out, d)
		}
	}
	return out, nil
}

// OpenIDs names these tasks, for a message that has to say WHICH work is holding a feature open.
func OpenIDs(tasks []store.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	return ids
}
