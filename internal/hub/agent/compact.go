// package: hub/agent / compact
// type:    logic (automatic context compaction)
// job:     fire Claude Code's own /compact into an agent's session at a leaf boundary, keeping a
// summary and dropping the transcript — the workflow engine's assignment gate (already holding the
// claim, -> workflow.Engine.prepareAssignment) decides when and for whom; this only performs it.
// limits:  the session, once; never the pod, worktree, or task queue, and never a second look at
// whether it landed below whatever threshold triggered it — that judgment is compactDue's, not
// this call's.
package agent

import (
	"fmt"

	"github.com/flo-at/sindri/internal/hub/workflow"
)

// Compact sends /compact into name's live session, then a kickoff to re-ask. Mirrors fireClear's
// mechanics — the boundary check, the liveness check, forgetting the stale reading — but queues
// rather than interrupts, so the claim's own directive already answers this ask, and the queued
// /compact+kickoff behind it just re-confirms it once it lands.
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
	if err := s.Inject(project, name, "/compact"); err != nil {
		return err
	}
	if err := s.Inject(project, name, workflow.MsgKickoff); err != nil {
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
