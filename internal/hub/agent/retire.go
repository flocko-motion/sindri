// package: hub/agent / retire
// type:    logic (winding an agent down)
// job:     set and clear an agent's retired flag — hand it no NEW work, while what it
// already holds runs to completion.
// limits:  the flag only; the assignment gate that reads it is the workflow's
// (-> workflow.claimNext), and stopping the pod stays a separate act.
package agent

import "fmt"

// SetRetired marks an agent to be given no further work, or clears that. It never touches what the
// agent is doing now: retiring is how a human winds one down without interrupting it, so the task in
// hand is finished and submitted first. Stopping the pod stays a separate, explicit act.
func (s *Service) SetRetired(project, name string, retired bool) error {
	ps := s.store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	a.Retired = retired
	if err := ps.PutAgent(a); err != nil {
		return err
	}
	defer s.deps.Notify()
	word := "retired: no new work"
	if !retired {
		word = "unretired: takes work again"
	}
	return ps.Log(name, "config", word)
}
