// package: hub/agent / compact
// type:    logic (automatic context compaction)
// job:     fire Claude Code's own /compact into an agent's session at a leaf boundary, keeping a
// summary and dropping the transcript — the assignment gate decides when this is due
// (-> workflow.Engine), this only fires it and logs what it fired on.
// limits:  the session only; it never touches the pod, worktree, or queue. Never mid-task, and
// never off the agent's own request — same hazard and same remedy as clear (-> FireArmedClears).
package agent

import (
	"fmt"
	"os"
)

// CompactDue reports whether name's live fill has grown past the point compacting it is worth the
// read it costs to produce a summary — the falling curve in CompactionThreshold, evaluated against
// the model it is CURRENTLY running. false with nothing recorded yet, same as ContextFull's rule.
func (s *Service) CompactDue(project, name string) (tokens int, due bool) {
	tokens, window, _, ok := s.ContextUsage(project, name)
	if !ok || window <= 0 {
		return tokens, false
	}
	return tokens, tokens >= s.CompactionThreshold(window)
}

// FireDueCompactions compacts every agent in a project whose fill has grown past its threshold and
// who has reached a leaf boundary — off the hub's tick, never the agent's own request, for the same
// reason FireArmedClears is: the injection interrupts the session, and an agent that just asked for
// work is mid-turn, holding the very command that would be cut off. Retired and clear-armed agents
// are skipped: retirement exempts every automatic behaviour, and an armed clear wins outright.
func (s *Service) FireDueCompactions(project string) {
	agents, err := s.store.For(project).Roster()
	if err != nil {
		return
	}
	for _, a := range agents {
		if a.Retired || a.ClearArmed {
			continue
		}
		if _, due := s.CompactDue(project, a.Name); !due {
			continue
		}
		at, err := s.AtLeafBoundary(project, a.Name)
		if err != nil || !at {
			continue
		}
		if err := s.compact(project, a.Name); err != nil {
			fmt.Fprintf(os.Stderr, "hub: compacting %s's context: %v\n", a.Name, err)
		}
	}
}

// compact sends /compact into name's live session. Mirrors fireClear's mechanics — the boundary
// re-check, the liveness check, forgetting the stale reading — since Claude Code's own compaction
// takes them unchanged; it differs only in never spending an armed flag (nothing arms this) and
// never re-serving the directive (a summary, unlike a wipe, leaves the agent knowing what it holds).
func (s *Service) compact(project, name string) error {
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
