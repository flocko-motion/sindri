// package: hub/agent / clearcontext
// type:    logic (the human-confirmed remedy for a full agent)
// job:     run Claude Code's own /clear inside a session and re-serve its directive — the
// follow-up to retirement (-> workflow.Engine.claimNext). Refuses while the agent
// holds a task: /clear would silently invalidate its file-tree memory.
// limits:  the session only; it never touches the pod, the worktree, or the task queue.
// Confirmation is the caller's (it executes, it doesn't ask).
package agent

import (
	"fmt"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// clearKickoffDelay lets Claude Code finish its own /clear reset (a redraw, not a cold boot) before
// the kickoff lands — shorter than rehydrate's launch wait, since the session is already live.
const clearKickoffDelay = 2 * time.Second

// ClearContext sends /clear into name's live session, then re-serves its directive so it picks up
// exactly where it would after a fresh launch (D13) — same session, empty context.
func (s *Service) ClearContext(project, name string) error {
	ps := s.store.For(project)
	st, err := ps.GetState(name)
	if err != nil {
		return err
	}
	if st.Task != "" || st.Container != "" {
		return fmt.Errorf("%s still holds %s — clearing only applies at a leaf boundary, once it's idle", name, dashOrTask(st))
	}
	if !s.AgentAlive(project, name) {
		return fmt.Errorf("agent %q is not running", name)
	}
	_ = s.Interrupt(project, name) // land on an idle prompt rather than queue behind a turn in flight
	if err := s.Inject(project, name, "/clear"); err != nil {
		return err
	}
	// Before the kickoff, not after: the kickoff makes the agent ask for work, and the answer is
	// computed from this measurement. Left standing it reports the size the clear just discarded, so
	// the agent is told it is still full — the exact remedy that had just been applied.
	s.ForgetContext(project, name)
	_ = ps.Log(name, "clear-context", "human-confirmed")
	go func() {
		time.Sleep(clearKickoffDelay)
		_ = s.InjectWhenReady(project, name, workflow.MsgKickoff)
	}()
	return nil
}

// dashOrTask names what st holds, for the refusal message: the container if it has one
// (the coarser unit a human would recognise), else the leaf task.
func dashOrTask(st store.AgentState) string {
	if st.Container != "" {
		return st.Container
	}
	return st.Task
}
