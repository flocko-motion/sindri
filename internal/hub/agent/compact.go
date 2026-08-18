// package: hub/agent / compact
// type:    logic (automatic context compaction)
// job:     fire Claude Code's own /compact into an agent's session at a leaf boundary, keeping a
// summary and dropping the transcript — the workflow engine's assignment gate decides when
// and for whom (-> workflow.Engine.claimNext), this only performs it and logs what it fired on.
// limits:  the session only; it never touches the pod, worktree, or queue. Never mid-task, and
// never off the agent's own request — same hazard and same remedy as clear (-> FireArmedClears).
package agent

import (
	"fmt"
)

// Compact sends /compact into name's live session. Mirrors fireClear's mechanics — the boundary
// re-check, the liveness check, forgetting the stale reading — since Claude Code's own compaction
// takes them unchanged; it differs only in never spending an armed flag (nothing arms this) and
// never re-serving the directive (a summary, unlike a wipe, leaves the agent knowing what it holds).
func (s *Service) Compact(project, name string) error {
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
	_ = s.Interrupt(project, name) // land on an idle prompt rather than queue behind a turn in flight
	if err := s.Inject(project, name, "/compact"); err != nil {
		return err
	}
	// Before the log line and the notify, not after: a reader of either must see the size compaction
	// was fired ON, not the fresh (and lower) one it left behind.
	s.ForgetContext(project, name)
	_ = s.store.For(project).Log(name, "compact", fmt.Sprintf(
		"fired at a leaf boundary: fill=%d window=%d model=%s threshold=%d recorded=%v", tokens, window, model, threshold, ok))
	s.deps.Notify()
	return nil
}
