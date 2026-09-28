// package: hub/harness / model
// type:    logic (choosing the model an agent runs on)
// job:     SetModel — the record and, for a live agent, the queued session switch: /clear (if
// there's anything to clear), then /model, then the next instruction.
// limits:  the record and the live switch; which model to choose is the caller's.
package harness

import (
	"context"
	"fmt"
	"time"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/hub/world/observe"
)

// SetModel changes the model an agent runs on, "" reverting to the account default. Not running:
// records the choice for the next Launch. Running: clears first (if needed), then injects /model,
// and blocks until both complete. Clearing first matters: /model on cached history shows a
// confirmation that silently drops whatever queues behind it (verified live).
func (s *Service) SetModel(ctx context.Context, project, name, model string) error {
	tell, err := s.chooseModel(ctx, project, name, model)
	if err != nil || !tell {
		return err
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

// SetTier puts an agent's session on the model tier dispatches to. It types the switch ALONE, with
// no clear of its own: the preparation step ahead of it empties the session, and a clear that did
// not land stops the agent there — so this never meets the cached history that would open a dialog.
// An unrecognised tier is refused rather than guessed at.
func (s *Service) SetTier(ctx context.Context, project, name, tier string) error {
	want, known := agentport.ModelForTier(tier)
	if !known {
		return fmt.Errorf("no model is mapped to tier %q — refusing to guess which one the work wants", tier)
	}
	tell, err := s.chooseModel(ctx, project, name, want)
	if err != nil || !tell {
		return err
	}
	return s.Inject(ctx, project, name, "/model "+want)
}

// chooseModel is the half both ways share: refuse a model the backend cannot size, record the choice,
// and report whether a live session still has to be told. It types nothing.
//
// Two facts, answered separately. The record is the CHOICE, written whenever it changes because the
// next launch carries it; the session is what RUNS, and only that says whether anything needs typing.
// Reading the record as proof of the session made every later switch a no-op that reported success.
func (s *Service) chooseModel(ctx context.Context, project, name, model string) (tell bool, err error) {
	if model != "" {
		if _, ok := s.ModelWindow(model); !ok {
			return false, fmt.Errorf("model %q has no known context window — refusing to start an agent whose fullness the hub cannot judge", model)
		}
	}
	ps := s.store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("no such agent %q", name)
	}
	old := a.Model
	live := s.AgentAlive(ctx, project, name)
	var detected string
	if live {
		_, _, detected, _ = s.ContextUsage(project, name)
	}
	onIt := s.ModelMatches(model, observe.ModelInUse(old, detected, live))
	if old != model {
		a.Model = model
		if err := ps.PutAgent(a); err != nil {
			return false, err
		}
		_ = ps.Log(name, "model", fmt.Sprintf("%s -> %s", modelLabel(old), modelLabel(model)))
		s.deps.Notify()
	}
	// Nothing live to retarget, no live command for the account default, or already there.
	return live && model != "" && !onIt, nil
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
