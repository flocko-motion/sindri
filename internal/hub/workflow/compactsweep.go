// package: hub/workflow / compactsweep
// type:    logic (firing a due compaction for every agent it applies to)
// job:     compactDue/containerCompactDue/reviewerCompactDue's decision, fired off the hub's tick
// rather than the agent's own request — same hazard and remedy as FireDueRetiers.
// limits:  which agent and whether it is due; the injection itself is Deps.Compact's
// (-> agent.Service.Compact).
package workflow

import (
	"fmt"
	"os"
)

// FireDueCompactions compacts every agent in a project whose fill has grown past its threshold AND
// who has an actual next assignment — task or review — waiting to justify it, at a leaf boundary.
// Retired and clear-armed agents are skipped: retirement exempts every automatic behaviour, and an
// armed clear wins outright. Planners and coauthors hold no assignable task or review to prepare
// for, so compaction never applies to them here.
func (e *Engine) FireDueCompactions(project string) {
	ps := e.store.For(project)
	roster, err := ps.Roster()
	if err != nil {
		return
	}
	for _, a := range roster {
		if a.Retired || a.ClearArmed || !e.deps.AgentAlive(project, a.Name) {
			continue
		}
		st, err := ps.GetState(a.Name)
		if err != nil {
			continue
		}
		var due bool
		switch a.Role {
		case "reviewer":
			held, err := ps.ReviewingPR(a.Name)
			if err != nil || held != "" {
				continue // mid-review: not a leaf boundary
			}
			_, due = e.reviewerCompactDue(project, a.Name)
		case "worker":
			if st.Task != "" {
				continue // mid-task: never here, whatever else is true
			}
			if st.Container != "" {
				_, due = e.containerCompactDue(project, a.Name, st.Container)
			} else {
				_, due = e.compactDue(project, a.Name)
			}
		default:
			continue
		}
		if !due {
			continue
		}
		if err := e.deps.Compact(project, a.Name); err != nil {
			fmt.Fprintf(os.Stderr, "hub: compacting %s's context: %v\n", a.Name, err)
		}
	}
}
