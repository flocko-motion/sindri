// package: hub/flow/task / approve_act
// type:    logic (the user's verdict on a proposed task)
// job:     approve or reject a task awaiting the user, and tell the planner either way
// — the gate that decides whether a proposal reaches the backlog.
// limits:  the gate's storage is hub/store's; who may set it is the caller's (both verdicts are
// the user's alone), and what an approved task then MEANS is the map beside this.
package task

import (
	"fmt"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"strings"

	"github.com/flo-at/sindri/internal/hub/task"
)

// ApproveTask clears the approval gate on a proposed task (user-only), making it claimable, and
// tells any running planner in the project. subtree takes every task below it that still awaits a
// verdict — a proposal usually arrives as a tree, and approving one node at a time is busywork.
func (a *Act) ApproveTask(project, id string, subtree bool) error {
	ps := a.Store.For(project)
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
	// Read before writing: the message describes a TRANSITION, so a second approval of an already-
	// approved id with nothing pending below it has nothing to announce.
	already, _ := ps.GetApproval(id)
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
	if already != "approved" || len(below) > 0 {
		a.notifyPlanners(project, fmt.Sprintf("[user] task %s%s was approved — it's now in the backlog for a worker.",
			id, withSubtasks(len(below))))
	}
	a.Deps.Notify()
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
func (a *Act) RejectTask(project, id, comment string) error {
	comment = strings.TrimSpace(comment)
	if comment == "" {
		comment = "rejected"
	}
	ps := a.Store.For(project)
	// Read before writing, same reason as ApproveTask: a repeat rejection with the same comment has
	// nothing new to say. A DIFFERENT comment is genuinely new feedback, so that still announces.
	prevStatus, prevComment := ps.GetApproval(id)
	if err := ps.SetApproval(id, "rejected", comment); err != nil {
		return err
	}
	if prevStatus != "rejected" || prevComment != comment {
		a.notifyPlanners(project, fmt.Sprintf("[user] task %s was rejected: %s", id, comment))
	}
	a.Deps.Notify()
	return nil
}

// notifyPlanners injects a message into every running planner's session in a project.
func (a *Act) notifyPlanners(project, msg string) {
	roster, _ := a.Store.For(project).Roster()
	for _, who := range roster {
		if who.Role == "planner" {
			name := who.Name
			go func() { _ = a.Harness.Say(project, name, msg, mail.MailAndPush) }()
		}
	}
}
