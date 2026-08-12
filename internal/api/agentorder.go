// package: api / agentorder
// type:    logic (roster display order)
// job:     the one order both front-ends must show the agent roster in: by repo
// path, then a deliberate role order, then name.
// limits:  a pure function over []AgentView; assembling the roster and its
// Projects list is the hub's.
package api

import "sort"

// agentRoleRank is the deliberate order agents list by within a repo: the path work actually takes
// through them, not alphabetical (which reads "coauthor, planner, reviewer, worker" and means
// nothing). An unrecognised role sorts last rather than panicking on a role this list doesn't know.
func agentRoleRank(role string) int {
	switch role {
	case "worker":
		return 0
	case "reviewer":
		return 1
	case "planner":
		return 2
	case "coauthor":
		return 3
	default:
		return 4
	}
}

// SortedAgents orders a roster for display: by repo (its registered path, so the Agents list groups
// repos in the same sequence the Repos list does — both read Projects, itself ordered by path),
// then role in agentRoleRank's fixed order, then name. Stable, so agents tying on all three keep a
// consistent order between refreshes rather than shuffling under the cursor. The one sort both CLI
// and TUI call, so `sindri agent list` and the Agents tab cannot drift onto different orders.
func SortedAgents(agents []AgentView, projects []Project) []AgentView {
	path := make(map[string]string, len(projects))
	for _, p := range projects {
		path[p.Tag] = p.Path
	}
	out := make([]AgentView, len(agents))
	copy(out, agents)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if pa, pb := path[a.Project], path[b.Project]; pa != pb {
			// An unregistered project (empty path) sorts last, not first — plain "" < x would put
			// the one repo the caller can't yet name ahead of every repo it can.
			if pa == "" || pb == "" {
				return pb == ""
			}
			return pa < pb
		}
		if ra, rb := agentRoleRank(a.Role), agentRoleRank(b.Role); ra != rb {
			return ra < rb
		}
		return a.Name < b.Name
	})
	return out
}
