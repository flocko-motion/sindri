// package: hub/agent / boundary
// type:    logic (the safe-to-disrupt read a caller consults before clearing or compacting)
// job:     AtLeafBoundary — whether the agent holds anything a session reset would cut into.
// limits:  the read alone; the rule behind it is the surface's (-> situation.Situation), and
// deciding whether to ask at all is the caller's (one inside an assignment it just claimed knows).
package agent

// AtLeafBoundary reports whether the agent holds nothing a clear or compaction would cut into: no
// leaf task, no PR awaiting a verdict, no review owed. A feature is not such a thing — between
// subtasks IS a boundary. Planners and coauthors are always at one.
func (s *Service) AtLeafBoundary(project, name string) (bool, error) {
	sit, err := s.sit.Of(project, name)
	if err != nil {
		return false, err
	}
	return sit.AtLeafBoundary(), nil
}
