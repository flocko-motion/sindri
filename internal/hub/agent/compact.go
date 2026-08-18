// package: hub/agent / compact
// type:    logic (automatic context compaction)
// job:     fire Claude Code's own /compact into an agent's session at a leaf boundary, keeping a
// summary and dropping the transcript — the workflow engine's assignment gate decides when
// and for whom (-> workflow.Engine.claimNext), this only performs it and logs what it fired on.
// limits:  the session and its own pending flag; never the pod, worktree, or task queue. Queues
// into the input stream rather than acting inline, so the caller ends its own turn right after
// firing (-> workflow.DirCompacting) instead of using what it just queued.
package agent

import (
	"fmt"
	"sync"

	"github.com/flo-at/sindri/internal/hub/workflow"
)

// compactPending marks, per agent, a fired compaction not yet observed to land — in memory only: a
// hub restart forgets it, and the worst that costs is one redundant /compact.
var compactPending struct {
	mu  sync.Mutex
	set map[string]bool
}

// CompactPending reports whether the gate should hold off firing again rather than stack a second
// /compact behind one still sitting in the queue (-> workflow.Engine.compactDue).
func (s *Service) CompactPending(project, name string) bool {
	compactPending.mu.Lock()
	defer compactPending.mu.Unlock()
	return compactPending.set[project+"/"+name]
}

// ForgetCompactPending clears the flag — called once a fresh ContextUsage reading is no longer due,
// the proof the queued compaction landed (or that none was ever pending, which is just as fine).
func (s *Service) ForgetCompactPending(project, name string) {
	compactPending.mu.Lock()
	defer compactPending.mu.Unlock()
	delete(compactPending.set, project+"/"+name)
}

// Compact sends /compact into name's live session, then a kickoff to re-ask. Mirrors fireClear's
// mechanics — the boundary re-check, the liveness check, forgetting the stale reading — but queues
// rather than interrupts: this call computes the reply to the agent's own in-flight directive
// request, and Escape used to kill that turn before the reply landed. Queued input survives a busy
// turn, even one that is interrupted (verified live), so both wait and run in order once the turn
// ends on its own — no kickoff delay needed either, since a summary leaves it able to re-ask at once.
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
	compactPending.mu.Lock()
	if compactPending.set == nil {
		compactPending.set = map[string]bool{}
	}
	compactPending.set[project+"/"+name] = true
	compactPending.mu.Unlock()
	_ = s.store.For(project).Log(name, "compact", fmt.Sprintf(
		"fired at a leaf boundary: fill=%d window=%d model=%s threshold=%d recorded=%v", tokens, window, model, threshold, ok))
	s.deps.Notify()
	return nil
}
