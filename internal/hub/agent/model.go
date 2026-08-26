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
// records the choice for the next Launch. Running: queues /clear (if there's anything to clear),
// /model, then next — no relaunch. Clearing first matters: /model on cached history shows a
// confirmation that silently drops whatever queues behind it (verified live).
func (s *Service) SetModel(ctx context.Context, project, name, model, next string) error {
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
		// A fresh session has nothing to discard, so the switch and its instruction go straight in.
		if err := s.Inject(ctx, project, name, "/model "+model); err != nil {
			return err
		}
		return s.Inject(ctx, project, name, next)
	}
	if err := s.Inject(ctx, project, name, "/clear"); err != nil {
		return err
	}
	// Before the wait, not after: next is composed from this measurement, and left standing it reports
	// the size the clear just discarded.
	s.ForgetContext(project, name)
	// The switch waits for the clear to have HAPPENED, not just for time to pass. /model opens a
	// confirmation dialog, and a dialog swallows whatever is typed behind it — so a /clear sent into
	// one is eaten, and the switch runs against the context the clear was meant to discard. Which is
	// backwards twice over: the old session is spent on the new model, and the reset lands after.
	s.kickoffWG.Add(1)
	go func() {
		defer s.kickoffWG.Done()
		if !s.awaitCleared(ctx, project, name, before) {
			return // the clear never took; switching now would spend the context it was to discard
		}
		if err := s.InjectWhenReady(ctx, project, name, "/model "+model); err != nil {
			return // logged as inject-skipped; sending next now would run it on the OLD model
		}
		_ = s.InjectWhenReady(ctx, project, name, next)
	}()
	return nil
}

// awaitCleared waits for the reading to FALL below before — /clear having happened, where a sleep
// only assumes it. A DROP is the test: context only grows within a session, and a fresh one carries
// a few tokens at once, so emptiness would never arrive. Sampled, since ForgetContext dropped memo.
func (s *Service) awaitCleared(ctx context.Context, project, name string, before int) bool {
	for waited := time.Duration(0); waited < clearSettleCap; waited += clearKickoffDelay {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(clearKickoffDelay):
		}
		if now, _, _, used := s.SampleContext(project, name); !used || now < before {
			return true
		}
	}
	// Never sent blind on timeout: the clear is most likely still QUEUED behind a long turn, and a
	// kickoff joining that queue is discarded by it — which is the silence this exists to prevent.
	// The stall and mail nudges are the backstop for an agent left idle.
	_ = s.store.For(project).Log(name, "clear-unconfirmed", "the /clear never took effect; nothing was sent after it")
	return false
}

// clearSettleCap bounds that wait. Long, because the clear waits out whatever turn was running when
// it was typed, and a review runs for minutes; bounded, because a goroutine per clear must end.
const clearSettleCap = 5 * time.Minute

// modelLabel names an empty model as the account default, for the log line.
func modelLabel(model string) string {
	if model == "" {
		return "(default)"
	}
	return model
}
