// package: hub/workflow / assign
// type:    logic (which open task is handed out next)
// job:     the order the backlog is worked in — one comparison across both claim
// pools, so a rating means the same thing whether it sits on a package or
// on a standalone task.
// limits:  the choice only; claiming it is task.go's (leaves) and feature.go's (packages),
// and which tasks are eligible at all is the store's (OpenLeaves / OpenContainers).
package workflow

import "github.com/flo-at/sindri/internal/hub/store"

// nextUp is the task the assigner hands out: the better-rated of the two pools' heads, ties by id,
// which is the store's own key extended across them. Taking one pool and only then the other made a
// rating comparable to some tasks and to nothing else — a mid package went out ahead of a critical
// one, and the backlog gave no account of why.
func nextUp(packages, leaves []store.Task) (t store.Task, isPackage, ok bool) {
	switch {
	case len(packages) == 0 && len(leaves) == 0:
		return store.Task{}, false, false
	case len(leaves) == 0:
		return packages[0], true, true
	case len(packages) == 0:
		return leaves[0], false, true
	}
	p, l := packages[0], leaves[0]
	if p.Priority != l.Priority {
		if p.Priority < l.Priority { // P0…P4: the lower code is the higher rating
			return p, true, true
		}
		return l, false, true
	}
	if l.ID < p.ID {
		return l, false, true
	}
	return p, true, true
}
