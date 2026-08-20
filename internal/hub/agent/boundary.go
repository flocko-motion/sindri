// package: hub/agent / boundary
// type:    logic (the safe-to-disrupt check Compact and FireClear both gate on)
// job:     AtLeafBoundary, and the one case reading the store alone gets wrong: a claim the gate
// just made, still preparing before the agent is told anything.
// limits:  the read and the flag correcting it; firing on the answer is Compact's or FireClear's.
package agent

import "sync"

// assigning marks, per agent, a gate-claimed assignment still preparing (a model switch or
// compaction) before the agent is told — nothing has happened under it yet, so AtLeafBoundary must
// admit it rather than refuse the very preparation it exists to gate. In memory only: a hub
// restart mid-preparation just costs one skipped step on the next claim.
var assigning struct {
	mu  sync.Mutex
	set map[string]bool
}

// BeginAssignment opens that window — call right after the claim, before any model switch or
// compaction, always paired with a deferred EndAssignment.
func (s *Service) BeginAssignment(project, name string) {
	assigning.mu.Lock()
	defer assigning.mu.Unlock()
	if assigning.set == nil {
		assigning.set = map[string]bool{}
	}
	assigning.set[project+"/"+name] = true
}

// EndAssignment closes it: preparation is done, so AtLeafBoundary reads the store again.
func (s *Service) EndAssignment(project, name string) {
	assigning.mu.Lock()
	defer assigning.mu.Unlock()
	delete(assigning.set, project+"/"+name)
}

// midAssignment is BeginAssignment's read, private since only AtLeafBoundary consults it.
func midAssignment(project, name string) bool {
	assigning.mu.Lock()
	defer assigning.mu.Unlock()
	return assigning.set[project+"/"+name]
}

// AtLeafBoundary reports whether the agent holds nothing a clear or compaction would cut into: no
// leaf task, no review owed, or a fresh claim still under BeginAssignment's window. A feature is
// not such a thing — between subtasks IS a boundary. Planners and coauthors are always at one.
func (s *Service) AtLeafBoundary(project, name string) (bool, error) {
	if midAssignment(project, name) {
		return true, nil
	}
	ps := s.store.For(project)
	st, err := ps.GetState(name)
	if err != nil {
		return false, err
	}
	if st.Task != "" {
		return false, nil
	}
	// A PR awaiting a verdict is not a boundary: the agent holds that task until it merges, and a
	// rejection returns the work mid-stream. Cutting the session there loses what it was waiting for.
	if pr, _, perr := ps.AwaitingPR(name); perr != nil || pr != "" {
		return false, perr
	}
	// store.Store's ReviewingPR, not ps's: a pooled reviewer's held review is never filed under
	// its own project, and a project-scoped read here would clear/compact it mid-review.
	_, reviewing, err := s.store.ReviewingPR(project, name)
	if err != nil {
		return false, err
	}
	return reviewing == "", nil
}
