// package: hub/harness / sleep
// type:    logic (whether an agent is empty-handed)
// job:     answer whether an agent holds nothing, which is what deciding to reclaim its pod turns on.
// limits:  the question. Reclaiming a pod and bringing one back are STATES the flow machine stands
// an agent in (-> roles/lifecycle's Stopping and Launching), and how long idle is too long is the
// flow's own dwell (-> flow.IdleStopThreshold).
package harness

// HoldsNothing is no leaf task, no held feature, no review owed, no open escalation, nobody dialed
// in — the question the idle reclaim asks, where AtLeafBoundary is the narrower one a session reset
// asks. The rule itself is the surface's (-> situation.Situation.HoldsNothing); role is taken as an
// argument still because a caller with the roster row in hand already knows it.
func (s *Service) HoldsNothing(project, name, role string) (bool, error) {
	if role == "coauthor" {
		return false, nil
	}
	sit, err := s.sit.Of(project, name)
	if err != nil {
		return false, err
	}
	return sit.HoldsNothing(), nil
}
