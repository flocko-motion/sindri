// package: hub/agent / compact
// type:    logic (automatic context compaction)
// job:     fire Claude Code's own /compact at a leaf boundary, then queue what to do once it
// lands — the assignment gate decides when, for whom, and with what; this only performs it.
// limits:  the session, once; never the pod, worktree, or task queue, and never a second look at
// whether it landed below whatever threshold triggered it — that judgment is compactDue's, not
// this call's.
package agent

import "fmt"

// Compact sends /compact into name's live session, then queues next behind it. Mirrors FireClear's
// mechanics otherwise, but never interrupts: every call runs inside the agent's own ask.
func (s *Service) Compact(project, name, next string) error {
	at, err := s.AtLeafBoundary(project, name)
	if err != nil {
		return err
	}
	if !at {
		return fmt.Errorf("%s is not at a leaf boundary — compaction only applies there", name)
	}
	if !s.AgentAlive(project, name) {
		return fmt.Errorf("agent %q is not running", name)
	}
	tokens, window, model, ok := s.ContextUsage(project, name)
	threshold := s.CompactionThreshold(window)
	if err := s.Inject(project, name, "/compact"); err != nil {
		return err
	}
	if err := s.Inject(project, name, next); err != nil {
		return err
	}
	// Before the log/notify: a reader must see the size compaction fired ON, not the fresh one after.
	s.ForgetContext(project, name)
	_ = s.store.For(project).Log(name, "compact", fmt.Sprintf(
		"fired at a leaf boundary: fill=%d window=%d model=%s threshold=%d recorded=%v", tokens, window, model, threshold, ok))
	s.deps.Notify()
	return nil
}
