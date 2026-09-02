// package: hub/agent / clearcontext
// type:    logic (a worker's context reset, armed by a human or fired by the assignment gate)
// job:     arm a context clear and fire it at the agent's next leaf boundary — Claude Code's
// own /clear inside the session, waited out until the session's context is seen to fall.
// Never mid-task: /clear would silently invalidate its file-tree memory.
// limits:  the flag and the session; it never touches the pod, the worktree, or the queue. What
// follows a clear is the caller's, and so is confirmation (it executes, it doesn't ask).
package agent

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/flo-at/sindri/internal/hub/workflow"
)

// clearSamplePeriod is how often a clear in flight is looked at again — long enough that Claude Code
// has finished its own reset (a redraw, not a cold boot) between two looks (-> awaitCleared).
const clearSamplePeriod = 2 * time.Second

// SetClearArmed arms a context clear, or takes it back. Arming is the whole decision a human makes:
// WHEN it lands is the agent's to say, so one at a leaf boundary is cleared now and one holding work
// keeps the arming until it reaches one (-> FireArmedClears). Disarming is just the flag.
func (s *Service) SetClearArmed(ctx context.Context, project, name string, armed bool) error {
	ps := s.store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	a.ClearArmed = armed
	if err := ps.PutAgent(a); err != nil {
		return err
	}
	if !armed {
		_ = ps.Log(name, "clear-context", "disarmed before it fired")
		s.deps.Notify()
		return nil
	}
	at, err := s.AtLeafBoundary(project, name)
	if err != nil {
		return err
	}
	if !at {
		_ = ps.Log(name, "clear-context", "armed: fires at its next leaf boundary")
		s.deps.Notify()
		return nil
	}
	if err := s.Clear(ctx, project, name); err != nil {
		// This call said "clears now" and could not. Undo the arming rather than leave a durable
		// flag behind an error the user reads as "nothing happened" — one that would also withhold
		// the agent from work. A failure in the SWEEP is the opposite case: the arming was set
		// deliberately, so it stands and tries again at the next boundary.
		_ = s.setArmed(project, name, false)
		s.deps.Notify()
		return err
	}
	_ = s.deps.Deliver(project, name, s.deps.Kickoff(project, name), workflow.PushOnly)
	return nil
}

// ClearArmed reports whether a clear is waiting to fire for this agent — what the assignment gate
// reads, since an armed agent must be handed no new leaf work before it lands.
func (s *Service) ClearArmed(project, name string) bool {
	a, ok, err := s.store.For(project).GetAgent(name)
	return err == nil && ok && a.ClearArmed
}

// FireArmedClears fires every armed clear in a project whose agent has reached a leaf boundary. Off
// the hub's tick rather than the agent's request: the clear itself never interrupts (that's the
// agent's ESC), and the kickoff lands afterwards.
func (s *Service) FireArmedClears(ctx context.Context, project string) {
	agents, err := s.store.For(project).Roster()
	if err != nil {
		return
	}
	for _, a := range agents {
		if !a.ClearArmed {
			continue
		}
		at, err := s.AtLeafBoundary(project, a.Name)
		if err != nil || !at {
			continue
		}
		if err := s.Clear(ctx, project, a.Name); err != nil {
			// The arming is SPENT by Clear, so a failure here would otherwise leave the agent
			// un-cleared with nothing left to fire again. Put back, and said on the agent's own record
			// rather than only on the hub's stderr, where nobody looking at the agent would find it.
			_ = s.setArmed(project, a.Name, true)
			_ = s.store.For(project).Log(a.Name, "clear-failed", err.Error()+" — still armed, retried at the next boundary")
			fmt.Fprintf(os.Stderr, "hub: clearing %s's context: %v\n", a.Name, err)
			continue
		}
		_ = s.deps.Deliver(project, a.Name, s.deps.Kickoff(project, a.Name), workflow.PushOnly)
	}
}

// Clear sends /clear into name's live session and blocks until it takes effect or times out. Whether
// this is a safe moment to discard the session is the caller's (-> AtLeafBoundary); a caller that has
// just claimed work for the agent is inside its own assignment and asks no such question.
// interruptSettle is how long the ESC is given to end the turn before the clear is typed. Short:
// this is a keystroke reaching a prompt, not work finishing.
const interruptSettle = 2 * time.Second

func (s *Service) Clear(ctx context.Context, project, name string) error {
	ps := s.store.For(project)
	if !s.deps.AgentUp(project, name) {
		return fmt.Errorf("agent %q is not running", name)
	}
	if err := s.setArmed(project, name, false); err != nil {
		return err
	}
	// INTERRUPTED FIRST. A /clear typed into a running turn is QUEUED, and cannot execute until that
	// turn ends — so a clear issued at a busy agent waited out its whole cap and then reported a
	// timeout, every time. ESC ends the turn, so the clear that follows runs immediately.
	//
	// Safe by construction rather than by luck: the only reason to clear is to discard what the
	// session holds, so cutting off the work in flight is what was being asked for anyway.
	if err := s.Interrupt(ctx, project, name); err != nil {
		return fmt.Errorf("could not interrupt %q before clearing: %w", name, err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(interruptSettle): // the turn has to end before the prompt will take input
	}
	before, _, _, _ := s.ContextUsage(project, name)
	if err := s.Inject(ctx, project, name, "/clear"); err != nil {
		return err
	}
	s.ForgetContext(project, name)
	_ = ps.Log(name, "clear-context", "fired")
	s.deps.Notify()
	if !s.awaitCleared(ctx, project, name, before) {
		return fmt.Errorf("context clear for %q timed out — session did not respond", name)
	}
	return nil
}

// setArmed writes the flag alone, leaving the rest of the roster row as it stands.
func (s *Service) setArmed(project, name string, armed bool) error {
	ps := s.store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil || !ok {
		return err
	}
	a.ClearArmed = armed
	return ps.PutAgent(a)
}
