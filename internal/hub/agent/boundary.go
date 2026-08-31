// package: hub/agent / boundary
// type:    logic (the safe-to-disrupt read a caller consults before clearing or compacting)
// job:     AtLeafBoundary — whether the agent holds anything a session reset would cut into.
// limits:  the read alone; acting on the answer, and deciding whether to ask at all, is the
// caller's (a caller inside an assignment it has just claimed already knows).
package agent

// AtLeafBoundary reports whether the agent holds nothing a clear or compaction would cut into: no
// leaf task and no review owed. A feature is not such a thing — between subtasks IS a boundary.
// Planners and coauthors are always at one.
func (s *Service) AtLeafBoundary(project, name string) (bool, error) {
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
