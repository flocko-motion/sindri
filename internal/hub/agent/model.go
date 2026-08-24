// package: hub/agent / model
// type:    logic (choosing the model an agent runs on)
// job:     SetModel — the record and, for a live agent, the queued session switch: /clear (if
// there's anything to clear), then /model, then the next instruction.
// limits:  the record and the live switch; which model to choose is the caller's.
package agent

import (
	"context"
	"fmt"
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
	if _, _, _, ok := s.ContextUsage(project, name); ok {
		if err := s.Inject(ctx, project, name, "/clear"); err != nil {
			return err
		}
	}
	if err := s.Inject(ctx, project, name, "/model "+model); err != nil {
		return err
	}
	if err := s.Inject(ctx, project, name, next); err != nil {
		return err
	}
	s.ForgetContext(project, name)
	return nil
}

// modelLabel names an empty model as the account default, for the log line.
func modelLabel(model string) string {
	if model == "" {
		return "(default)"
	}
	return model
}
