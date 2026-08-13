// package: hub/agent / clearcontext
// type:    logic (the human-confirmed remedy for a full agent)
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

// SetClearArmed arms a context clear, or takes it back. Arming is the whole decision a human makes:
// WHEN it lands is the agent's to say, so one at a leaf boundary is cleared now and one holding work
// keeps the arming until it reaches one (-> FireArmedClears). Disarming is just the flag.
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
	return s.fireClear(project, name)
}

// ClearArmed reports whether a clear is waiting to fire for this agent — what the assignment gate
// reads, since an armed agent must be handed no new leaf work before it lands.
func (s *Service) ClearArmed(project, name string) bool {
	a, ok, err := s.store.For(project).GetAgent(name)
	return err == nil && ok && a.ClearArmed
}

// AtLeafBoundary reports whether the agent holds nothing a clear would cut into: no leaf task, no
// review owed. A feature is not such a thing — between subtasks IS a boundary, and the next
// subtask's directive names the feature afresh. Planners and coauthors are always at one.
func (s *Service) AtLeafBoundary(project, name string) (bool, error) {
	ps := s.store.For(project)
	st, err := ps.GetState(name)
	if err != nil {
		return false, err
	}
	if st.Task != "" {
		return false, nil
	}
	reviewing, err := ps.ReviewingPR(name)
	if err != nil {
		return false, err
	}
	return reviewing == "", nil
}

// FireArmedClears fires every armed clear in a project whose agent has reached a leaf boundary. Off
// the hub's tick rather than the agent's request: the clear interrupts the session, and an agent
// that just asked for work is mid-turn, holding the very command that would be cut off.
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
		if err := s.fireClear(project, a.Name); err != nil {
			fmt.Fprintf(os.Stderr, "hub: clearing %s's context: %v\n", a.Name, err)
		}
	}
}

// fireClear sends /clear into name's live session, then re-serves its directive so it picks up where
// it would after a fresh launch (D13) — same session, empty context. The arming is spent here
// whatever follows: one left standing would fire again at every boundary.
func (s *Service) fireClear(project, name string) error {
	ps := s.store.For(project)
	at, err := s.AtLeafBoundary(project, name)
	if err != nil {
		return err
	}
	if !at {
		st, _ := ps.GetState(name)
		return fmt.Errorf("%s still holds %s — clearing only applies at a leaf boundary", name, dashOrTask(st))
	}
	if !s.AgentAlive(project, name) {
		return fmt.Errorf("agent %q is not running", name)
	}
	if err := s.setArmed(project, name, false); err != nil {
		return err
	}
	_ = s.Interrupt(project, name) // land on an idle prompt rather than queue behind a turn in flight
	if err := s.Inject(project, name, "/clear"); err != nil {
		return err
	}
	// Before the kickoff, not after: the kickoff makes the agent ask for work, and the answer is
	// computed from this measurement. Left standing it reports the size the clear just discarded, so
	// the agent is told it is still full — the exact remedy that had just been applied.
	s.ForgetContext(project, name)
	_ = ps.Log(name, "clear-context", "fired at a leaf boundary")
	s.deps.Notify()
	go func() {
		time.Sleep(clearKickoffDelay)
		_ = s.InjectWhenReady(project, name, workflow.MsgKickoff)
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
