// package: api / priority
// type:    logic (a wire value + the pure rule behind it)
// job:     how far a priority setting reaches (PriorityScope), and what reaching below a task
// would ACTUALLY do to the tasks down there (PriorityEffect) — the fact both front-ends
// state before offering the choice.
// limits:  data and pure derivation only; applying a scope is the hub's (-> workflow.SetPriority)
// and the wording is a front-end's (-> internal/ui/theme).
package api

// PriorityScope is how far a priority setting reaches below the task it names.
type PriorityScope string

const (
	ScopeTask    PriorityScope = "task"    // the named task alone
	ScopeUnrated PriorityScope = "unrated" // and every open task below it carrying no priority
	ScopeAll     PriorityScope = "all"     // and every open task below it, overwriting what is set
)

// PriorityScopes are the three, narrowest first — the order a menu offers them in.
var PriorityScopes = []PriorityScope{ScopeTask, ScopeUnrated, ScopeAll}

// ParsePriorityScope reads a scope off the wire or a flag; "" is the narrowest, so a caller that
// knows nothing of scopes keeps the single-task behaviour. ok is false for anything else, which a
// caller reports rather than silently widening or narrowing what the user asked for.
func ParsePriorityScope(s string) (PriorityScope, bool) {
	if s == "" {
		return ScopeTask, true
	}
	for _, sc := range PriorityScopes {
		if PriorityScope(s) == sc {
			return sc, true
		}
	}
	return ScopeTask, false
}

// PriorityCascade is what a priority set on one task would reach below it, and what reaching there
// would do. Independent is the whole reason the distinction is drawn: see PriorityEffect.
type PriorityCascade struct {
	Children    int  // open tasks below it, at any depth
	Unrated     int  // those of them carrying no priority of their own
	Independent bool // they will be claimed one by one rather than as part of this package
}

// PriorityEffect reports what rating the tasks below id would do, which is NOT one answer.
//
// A package is claimed as one unit, so while the parent is open its own rating releases the whole tree
// and a child's only orders the subtasks within it. Once the parent has ENDED each open child stands
// alone, with nothing above it to be claimed instead, and its own rating is what makes it claimable.
//
// Hence Independent = the parent has ended. Rating below an open parent reorders; below an ended one it
// releases. One wording for both would claim to have released work it had merely reordered.
func PriorityEffect(tasks []Task, id string) PriorityCascade {
	var c PriorityCascade
	for _, t := range tasks {
		if t.ID == id {
			c.Independent = Done(t)
			break
		}
	}
	for _, d := range Descendants(tasks, id) {
		if !Open(d) {
			continue // a finished task's priority decides nothing, so a cascade never touches it
		}
		c.Children++
		if d.Priority == "" {
			c.Unrated++
		}
	}
	return c
}
