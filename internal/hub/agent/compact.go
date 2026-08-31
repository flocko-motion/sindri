// package: hub/agent / compact
// type:    logic (automatic context compaction)
// job:     fire Claude Code's own /compact and wait it out — the assignment gate decides when,
// for whom, and what follows; this only performs it and answers.
// limits:  the session, once; never the pod, worktree, or task queue, and never a second look at
// whether it landed below whatever threshold triggered it — that judgment is compactDue's, not
// this call's.
package agent

import (
	"context"
	"fmt"
)

// Compact sends /compact into name's live session and blocks until it takes effect or times out.
// Whether this is a safe moment to summarise the session away is the caller's (-> AtLeafBoundary).
// Returns an error if compaction never settles or injection fails.
func (s *Service) Compact(ctx context.Context, project, name string) error {
	if !s.AgentAlive(ctx, project, name) {
		return fmt.Errorf("agent %q is not running", name)
	}
	tokens, window, model, ok := s.ContextUsage(project, name)
	before := tokens
	threshold := s.CompactionThreshold(window)
	if err := s.Inject(ctx, project, name, "/compact"); err != nil {
		return err
	}
	s.ForgetContext(project, name)
	if !s.awaitCleared(ctx, project, name, before) {
		return fmt.Errorf("context compaction for %q timed out — session did not respond", name)
	}
	_ = s.store.For(project).Log(name, "compact", fmt.Sprintf(
		"fired: fill=%d window=%d model=%s threshold=%d recorded=%v", tokens, window, model, threshold, ok))
	s.deps.Notify()
	return nil
}
