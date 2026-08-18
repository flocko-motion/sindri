// package: hub/agent / model
// type:    logic (choosing the model an agent runs on)
// job:     SetModel — launching on a chosen model and changing a running agent's are the same act,
// since a session belongs to its old model and can't cross onto a new one without losing it.
// limits:  the record and the relaunch; which model to choose is the caller's.
package agent

import (
	"fmt"
	"io"
)

// SetModel changes the model an agent runs on, "" reverting to the account default. A non-empty
// model must resolve through the backend's own window table (-> ModelWindow), or its fullness
// would be unjudgeable. Not running: just records the choice. Running: compacts the old session
// first, since a bare swap would lose it, then relaunches on the new model.
func (s *Service) SetModel(project, name, model string) error {
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
	if !s.AgentAlive(project, name) {
		return nil // nothing live to compact; the next Launch starts on the new model
	}
	// No recorded usage means a fresh session — nothing to carry across, so nothing to compact.
	if _, _, _, ok := s.ContextUsage(project, name); ok {
		// The restart right after kills this turn regardless, so unlike Compact's other callers an
		// interrupt here costs nothing extra — it just gives the queued /compact its usual immediate start.
		_ = s.Interrupt(project, name)
		if err := s.Compact(project, name); err != nil {
			return fmt.Errorf("compacting %s before its model change: %w", name, err)
		}
	}
	return s.RestartAgent(project, name, io.Discard)
}

// modelLabel names an empty model as the account default, for the log line.
func modelLabel(model string) string {
	if model == "" {
		return "(default)"
	}
	return model
}
