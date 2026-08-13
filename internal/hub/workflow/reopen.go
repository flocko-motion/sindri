// package: hub/workflow / reopen
// type:    logic (task reopen)
// job:     restore a closed sindri-owned task to "open" — the one thing close.go
// cannot undo on its own, for the task closed before it actually held.
// limits:  sindri-owned ids only; recording why is the caller's (-> hub.ReopenTask,
// which also writes the task comment, so both its entry points must supply a reason).
package workflow

import (
	"fmt"

	"github.com/flo-at/sindri/internal/hub/task"
)

// ReopenTask restores a closed sindri-owned task to "open". Refused for a gh-/os- id, whose status
// comes from its own source rather than sindri's store, and for a task that isn't closed, since
// there is nothing to reopen.
//
// Deliberately NO approval gate: this restores the release a closed task already had rather than
// granting new work, so it must not cost more than filing a duplicate would. Priority is left
// exactly as it stood, which alone can make the task claimable again — the caller says so.
func (e *Engine) ReopenTask(project, id string) error {
	if !task.IsOwned(id) {
		return fmt.Errorf("%s is not a task sindri owns — its status comes from its own source "+
			"(an openspec change or a GitHub issue), not sindri's store, so it can't be reopened here", id)
	}
	ps := e.store.For(project)
	t, ok, err := ps.OwnedTask(id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("this project owns no such task %q", id)
	}
	if t.Status != "closed" {
		return fmt.Errorf("%s is %s, not closed — there is nothing to reopen", id, t.Status)
	}
	// Through SetStatus, not the store directly — the one place allowed to write owned status.
	if err := e.SetStatus(project, id, "open"); err != nil {
		return err
	}
	if err := e.RefreshTask(project, id); err != nil {
		return err
	}
	e.deps.Notify()
	return nil
}
