// package: api / prorder
// type:    logic (PR display order)
// job:     the one order both front-ends list pull requests in: by repo, then
// whatever order they arrived in (newest first, from the store).
// limits:  a pure function over []PR; assembling the list and its Projects is the hub's.
package api

import "sort"

// SortedPRs orders pull requests for display: by repo (its registered path, the same key
// SortedAgents uses, so both lists group repos in the sequence the Repos list does), and within a
// repo by nothing at all — the sort is STABLE, so the store's newest-first order survives.
//
// Grouping is what makes a foreign row legible. Repo scope shows PRs from elsewhere when they wait
// on the user, and interleaved by age they read as though the scope had simply stopped working;
// gathered under their own repo they read as what they are. The one sort both CLI and TUI call, so
// `sindri pr list` and the PRs tab cannot drift onto different orders.
func SortedPRs(prs []PR, projects []Project) []PR {
	path := make(map[string]string, len(projects))
	for _, p := range projects {
		path[p.Tag] = p.Path
	}
	out := make([]PR, len(prs))
	copy(out, prs)
	sort.SliceStable(out, func(i, j int) bool {
		pa, pb := path[out[i].Project], path[out[j].Project]
		if pa == pb {
			return false
		}
		// An unregistered project (empty path) sorts last, not first — plain "" < x would put the
		// one repo the caller cannot yet name ahead of every repo it can.
		if pa == "" || pb == "" {
			return pb == ""
		}
		return pa < pb
	})
	return out
}
