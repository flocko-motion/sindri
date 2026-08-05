// package: hub/workflow / approval
// type:    logic (the user's verdict on a proposed task)
// job:     approve or reject a task awaiting the user, and tell the planner either way
// — the gate that decides whether a proposal reaches the backlog.
// limits:  the gate's storage is hub/store's; who may set it is the caller's (both
// verdicts are the user's alone).
package workflow

import (
	"fmt"
	"strings"
)

// ApproveTask clears the approval gate on a proposed task (user-only), making it claimable, and
// tells any running planner in the project.
func (e *Engine) ApproveTask(project, id string) error {
	if err := e.store.For(project).SetApproval(id, "approved", ""); err != nil {
		return err
	}
	e.notifyPlanners(project, fmt.Sprintf("[user] task %s was approved — it's now in the backlog for a worker.", id))
	e.deps.Notify()
	return nil
}

// RejectTask rejects a proposed task with a comment (user-only); it stays hidden from workers, and
// the comment is delivered to any running planner.
func (e *Engine) RejectTask(project, id, comment string) error {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		comment = "rejected"
	}
	if err := e.store.For(project).SetApproval(id, "rejected", comment); err != nil {
		return err
	}
	e.notifyPlanners(project, fmt.Sprintf("[user] task %s was rejected: %s", id, comment))
	e.deps.Notify()
	return nil
}

// notifyPlanners injects a message into every running planner's session in a project.
func (e *Engine) notifyPlanners(project, msg string) {
	roster, _ := e.store.For(project).Roster()
	for _, a := range roster {
		if a.Role == "planner" {
			name := a.Name
			go func() { _ = e.deps.InjectWhenReady(project, name, msg) }()
		}
	}
}
