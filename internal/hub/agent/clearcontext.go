// package: hub/agent / clearcontext
// type:    logic (a worker's context reset, armed by a human or fired by the assignment gate)
// job:     arm a context clear and fire it at the agent's next leaf boundary — Claude Code's
// own /clear inside the session, then its directive re-served (-> workflow.claimNext).
// Never mid-task: /clear would silently invalidate its file-tree memory.
// limits:  the flag and the session; it never touches the pod, the worktree, or the queue.
// Confirmation is the caller's (it executes, it doesn't ask).
package agent

import (
	"fmt"
	"os"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// clearKickoffDelay lets Claude Code finish its own /clear reset (a redraw, not a cold boot) before
// the kickoff lands — shorter than rehydrate's launch wait, since the session is already live.
const clearKickoffDelay = 2 * time.Second

// SetClearArmed arms a context clear, or takes it back. WHEN it lands is the agent's to say: one at
// a leaf boundary clears now, one holding work waits for FireArmedClears. Disarming is just the flag.
func (s *Service) SetClearArmed(project, name string, armed bool) error {
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
	if err := s.FireClear(project, name, workflow.MsgKickoff, true); err != nil {
		// Undo the arming rather than leave it behind an error the user reads as "nothing happened".
		// A sweep failure is the opposite case: the arming stands and tries again at the next boundary.
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

// FireArmedClears fires every armed clear in a project whose agent has reached a leaf boundary — off
// the hub's tick, where nothing of the agent's own is in flight to interrupt.
func (s *Service) FireArmedClears(project string) {
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
		if err := s.FireClear(project, a.Name, workflow.MsgKickoff, true); err != nil {
			fmt.Fprintf(os.Stderr, "hub: clearing %s's context: %v\n", a.Name, err)
		}
	}
}

// FireClear sends /clear into name's live session, then queues next behind it on a delay. interrupt
// is true where nothing of the agent's is in flight (a terminal, or the hub's own tick) — never
// where the call answers the agent's own ask, which ESC would cut off mid-turn.
func (s *Service) FireClear(project, name, next string, interrupt bool) error {
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
		_ = s.Interrupt(project, name)
	}
	if err := s.Inject(project, name, "/clear"); err != nil {
		return err
	}
	// Before the kickoff, not after: next is computed from this measurement, and left standing it
	// reports the size the clear just discarded — telling a cleared agent it is still full.
	s.ForgetContext(project, name)
	_ = ps.Log(name, "clear-context", "fired at a leaf boundary")
	s.deps.Notify()
	go func() {
		time.Sleep(clearKickoffDelay)
		_ = s.InjectWhenReady(project, name, next)
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
