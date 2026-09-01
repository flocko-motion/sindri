// package: hub/agent / model
// type:    logic (choosing the model an agent runs on)
// job:     SetModel — the record and, for a live agent, the queued session switch: /clear (if
// there's anything to clear), then /model, then the next instruction.
// limits:  the record and the live switch; which model to choose is the caller's.
package agent

import (
	"context"
	"fmt"
	"time"
)

// SetModel changes the model an agent runs on, "" reverting to the account default. Not running:
// records the choice for the next Launch. Running: clears first (if needed), then injects /model,
// and blocks until both complete. Clearing first matters: /model on cached history shows a
// confirmation that silently drops whatever queues behind it (verified live).
func (s *Service) SetModel(ctx context.Context, project, name, model string) error {
	if model != "" {
		if _, ok := s.ModelWindow(model); !ok {
			return fmt.Errorf("model %q has no known context window — refusing to start an agent whose fullness the hub cannot judge", model)
		}
	}
	ps := s.store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	old := a.Model
	if old == model {
		return nil // already there; nothing to disturb
	}
	a.Model = model
	if err := ps.PutAgent(a); err != nil {
		return err
	}
	_ = ps.Log(name, "model", fmt.Sprintf("%s -> %s", modelLabel(old), modelLabel(model)))
	s.deps.Notify()
	if !s.AgentAlive(ctx, project, name) || model == "" {
		return nil // nothing live to retarget, or no live command yet for the account default
	}
	before, _, _, used := s.ContextUsage(project, name)
	if !used {
		// A fresh session has nothing to discard, so the switch goes straight in.
		return s.Inject(ctx, project, name, "/model "+model)
	}
	// Clear first: /model on cached history shows a dialog that swallows whatever follows, so the
	// clear must land before /model.
	if err := s.Inject(ctx, project, name, "/clear"); err != nil {
		return err
	}
	s.ForgetContext(project, name)
	if !s.awaitCleared(ctx, project, name, before) {
		return fmt.Errorf("context clear for %q timed out before model switch — session did not respond", name)
	}
	return s.Inject(ctx, project, name, "/model "+model)
}

// awaitCleared waits for the reading to FALL below before — /clear having happened, where a sleep
// only assumes it. A DROP is the test: context only grows within a session, and a fresh one carries
// a few tokens at once, so emptiness would never arrive. Sampled, since ForgetContext dropped memo.
func (s *Service) awaitCleared(ctx context.Context, project, name string, before int) bool {
	for waited := time.Duration(0); waited < clearSettleCap; waited += clearSamplePeriod {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(clearSamplePeriod):
		}
		if now, _, _, used := s.SampleContext(project, name); !used || now < before {
			return true
		}
	}
	// Logged as well as returned: the caller decides what happens next, and this leaves the evidence
	// on the agent's own record where whoever reads the failure later goes looking.
	_ = s.store.For(project).Log(name, "clear-unconfirmed", "the /clear never took effect within the wait")
	return false
}

// clearSettleCap bounds that wait. Long, because the clear waits out whatever turn was running when
// it was typed, and a review runs for minutes; bounded, because its caller is blocked on the answer.
const clearSettleCap = 5 * time.Minute

// modelLabel names an empty model as the account default, for the log line.
func modelLabel(model string) string {
	if model == "" {
		return "(default)"
	}
	return model
}
