// package: hub/agent / clearcontext
// type:    logic (a worker's context reset, armed by a human or fired by the assignment gate)
// job:     arm a context clear and fire it at the agent's next leaf boundary — Claude Code's
// own /clear inside the session, then its directive re-served (-> workflow.Engine.Kickoff).
// Never mid-task: /clear would silently invalidate its file-tree memory.
// limits:  the flag and the session; it never touches the pod, the worktree, or the queue.
// Confirmation is the caller's (it executes, it doesn't ask).
package agent

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
)

// clearKickoffDelay lets Claude Code finish its own /clear reset (a redraw, not a cold boot) before
// the kickoff lands — shorter than rehydrate's launch wait, since the session is already live.
const clearKickoffDelay = 2 * time.Second

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
	if err := s.FireClear(ctx, project, name, s.deps.Kickoff(project, name), true); err != nil {
		// This call said "clears now" and could not. Undo the arming rather than leave a durable
		// flag behind an error the user reads as "nothing happened" — one that would also withhold
		// the agent from work. A failure in the SWEEP is the opposite case: the arming was set
		// deliberately, so it stands and tries again at the next boundary.
		_ = s.setArmed(project, name, false)
		s.deps.Notify()
		return err
	}
	return nil
}

// ClearArmed reports whether a clear is waiting to fire for this agent — what the assignment gate
// reads, since an armed agent must be handed no new leaf work before it lands.
func (s *Service) ClearArmed(project, name string) bool {
	a, ok, err := s.store.For(project).GetAgent(name)
	return err == nil && ok && a.ClearArmed
}

// FireArmedClears fires every armed clear in a project whose agent has reached a leaf boundary. Off
// the hub's tick rather than the agent's request: the clear interrupts the session, and an agent
// that just asked for work is mid-turn, holding the very command that would be cut off.
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
		if err := s.FireClear(ctx, project, a.Name, s.deps.Kickoff(project, a.Name), true); err != nil {
			fmt.Fprintf(os.Stderr, "hub: clearing %s's context: %v\n", a.Name, err)
		}
	}
}

// FireClear sends /clear into name's live session, then queues next behind it on a delay. interrupt
// is true where nothing of the agent's is in flight (a terminal, or the hub's own tick) — never
// where the call answers the agent's own ask, which ESC would cut off mid-turn. The arming is spent
// before the injection, so a failed inject loses it rather than firing again at every boundary.
func (s *Service) FireClear(ctx context.Context, project, name, next string, interrupt bool) error {
	ps := s.store.For(project)
	at, err := s.AtLeafBoundary(project, name)
	if err != nil {
		return err
	}
	if !at {
		st, _ := ps.GetState(name)
		return fmt.Errorf("%s still holds %s — clearing only applies at a leaf boundary", name, dashOrTask(st))
	}
	if !s.deps.AgentUp(project, name) {
		return fmt.Errorf("agent %q is not running", name)
	}
	if err := s.setArmed(project, name, false); err != nil {
		return err
	}
	if interrupt {
		_ = s.Interrupt(ctx, project, name)
	}
	// Read BEFORE the injection: it is the figure the kickoff waits to see fall (-> awaitCleared).
	before, _, _, _ := s.ContextUsage(project, name)
	if err := s.Inject(ctx, project, name, "/clear"); err != nil {
		return err
	}
	// Before the kickoff, not after: next is computed from this measurement, and left standing it
	// reports the size the clear just discarded — telling a cleared agent it is still full.
	s.ForgetContext(project, name)
	_ = ps.Log(name, "clear-context", "fired at a leaf boundary")
	s.deps.Notify()
	// The kickoff waits out the clear, so it runs on ctx rather than on the caller's return: every
	// caller hands work-lifetime context here — a handler detaches from its request, the sweeps carry
	// the hub's own — and one that does not means to abandon this too. kickoffWG lets a test join it
	// (-> waitForKickoff) rather than tearing its fixture down while this is still in flight.
	s.kickoffWG.Add(1)
	go func() {
		defer s.kickoffWG.Done()
		// Waits for the clear to have HAPPENED, never for time to pass: a /clear typed mid-turn is
		// queued, and discards the queue it is in — so a kickoff on a timer joined it and was lost.
		if !s.awaitCleared(ctx, project, name, before) {
			return
		}
		_ = s.InjectWhenReady(ctx, project, name, next)
	}()
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

// dashOrTask names what st holds, for the refusal message: the container if it has one, else the task.
func dashOrTask(st store.AgentState) string {
	if st.Container != "" {
		return st.Container
	}
	return st.Task
}
