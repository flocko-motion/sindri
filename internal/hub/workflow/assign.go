// package: hub/workflow / assign
// type:    logic (which open task is handed out next)
// job:     the order the backlog is worked in — one comparison across both claim
// pools, so a rating means the same thing whether it sits on a package or
// on a standalone task.
// limits:  the choice only; claiming it is task.go's (leaves) and feature.go's (packages),
// and which tasks are eligible at all is the store's (OpenLeaves / OpenContainers).
package workflow

import (
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// nextUp is the task the assigner hands out: the better-rated of the two pools' heads, ties by id
// — the store's own key, extended across them so a rating means the same thing on either. prefers,
// when not nil, breaks that tie toward avoiding a model change, but only within the BEST priority
// present: it moves which of several equally-rated tasks goes first, never reaches past a
// higher-rated one, since priority is the user's flow control, not the fleet's to spend.
func nextUp(packages, leaves []store.Task, prefers func(store.Task) bool) (t store.Task, isPackage, ok bool) {
	if len(packages) == 0 && len(leaves) == 0 {
		return store.Task{}, false, false
	}
	topPackages, topLeaves := bestPriority(packages), bestPriority(leaves)
	// Each pool's own top slice can be a priority behind the other's, so it must be narrowed to the
	// BEST PRIORITY ACROSS BOTH before prefers ever sees it — otherwise a preferred task at the top
	// of its own, worse-rated pool would be scanned as if it were tied with the other pool's best.
	switch {
	case len(topPackages) == 0 || len(topLeaves) == 0:
		// one pool empty: the other's top slice is already the best present
	case topPackages[0].Priority < topLeaves[0].Priority: // P0…P4: the lower code is the higher rating
		topLeaves = nil
	case topLeaves[0].Priority < topPackages[0].Priority:
		topPackages = nil
	}
	if prefers != nil {
		for _, p := range topPackages {
			if prefers(p) {
				return p, true, true
			}
		}
		for _, l := range topLeaves {
			if prefers(l) {
				return l, false, true
			}
		}
	}
	switch {
	case len(topLeaves) == 0:
		return topPackages[0], true, true
	case len(topPackages) == 0:
		return topLeaves[0], false, true
	case topLeaves[0].ID < topPackages[0].ID:
		return topLeaves[0], false, true
	default:
		return topPackages[0], true, true
	}
}

// tierPrefers builds nextUp's tiebreak for one agent: a task whose tier's model matches what it is
// currently running, so a tie on priority is resolved toward avoiding a model change. "" for agent
// (a hypothetical role, no one specific to avoid a change for) answers with no preference at all.
func (e *Engine) tierPrefers(project, agent string) func(store.Task) bool {
	if agent == "" {
		return nil
	}
	current := e.deps.CurrentModel(project, agent)
	return func(t store.Task) bool {
		want, ok := e.deps.ModelForTier(api.TierOrDefault(t.Tier))
		return ok && want == current
	}
}

// bestPriority is the prefix of a priority-sorted pool sharing its best (lowest P-code) rating —
// the tied set nextUp's tiebreak may choose within, never past.
func bestPriority(sorted []store.Task) []store.Task {
	if len(sorted) == 0 {
		return nil
	}
	best := sorted[0].Priority
	i := 1
	for i < len(sorted) && sorted[i].Priority == best {
		i++
	}
	return sorted[:i]
}
