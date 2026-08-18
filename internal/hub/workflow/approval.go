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

	"github.com/flo-at/sindri/internal/hub/task"
)

// ApproveTask clears the approval gate on a proposed task (user-only), making it claimable, and
// tells any running planner in the project. subtree takes every task below it that still awaits a
// verdict — a proposal usually arrives as a tree, and approving one node at a time is busywork.
func (e *Engine) ApproveTask(project, id string, subtree bool) error {
	ps := e.store.For(project)
	var below []string
	if subtree {
		all, err := ps.AllTasks()
		if err != nil {
			return err
		}
		for _, d := range task.PendingApproval(all, id) {
			below = append(below, d.ID)
		}
	}
	// Children before the parent: the parent turning claimable is what releases the package, and a
	// worker claiming between the two writes would find a package half of which it cannot see.
	for _, child := range below {
		if err := ps.SetApproval(child, "approved", ""); err != nil {
			return fmt.Errorf("approve %s: %w", child, err)
		}
	}
	if err := ps.SetApproval(id, "approved", ""); err != nil {
		return err
	}
	e.notifyPlanners(project, fmt.Sprintf("[user] task %s%s was approved — it's now in the backlog for a worker.",
		id, withSubtasks(len(below))))
	e.deps.Notify()
	return nil
}

// withSubtasks names how far an approval reached, for the planner's message.
func withSubtasks(n int) string {
	switch n {
	case 0:
		return ""
	case 1:
		return " and its 1 subtask"
	}
	return fmt.Sprintf(" and its %d subtasks", n)
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
			go func() { _ = e.deps.Deliver(project, name, msg, MailAndPush) }()
		}
	}
}
