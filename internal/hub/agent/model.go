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
// would be unjudgeable. Not running: just records the choice. Running: clears the old session
// first, then relaunches on the new model.
//
// Clears rather than compacts: at a model change the agent holds nothing (its own boundary check
// runs exactly here), the context was produced by the OLD model's reasoning, and a downgrade's
// window can be smaller than even a well-compacted transcript can fit — arithmetically impossible
// to carry across. FireClear's own re-served kickoff also answers "and then what" for the pod that
// comes back up, which a bare compact never did.
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
		return nil // nothing live to clear; the next Launch starts fresh on the new model
	}
	// No recorded usage means a fresh session — nothing there for /clear to do.
	if _, _, _, ok := s.ContextUsage(project, name); ok {
		if err := s.FireClear(project, name); err != nil {
			return fmt.Errorf("clearing %s before its model change: %w", name, err)
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
