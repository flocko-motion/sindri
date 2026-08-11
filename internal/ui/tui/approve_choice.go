// package: tui / approve choice
// type:    ui (Tasks tab confirm modal)
// job:     the approve confirm for a proposed task that has a tree under it — it counts the
// tasks below still awaiting a verdict and offers the wider approve, so a package reaches the
// backlog whole in one keypress — and hands straight on to the priority, the second gate an
// approve alone leaves shut.
// limits:  labels and key-to-call plumbing only; which tasks a verdict may reach is the hub's
// (-> api.PendingApproval) and the cascade itself is ApproveTask's.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// pendingBelow counts the tasks under id that still await the user's verdict.
func (m model) pendingBelow(id string) int {
	return len(api.PendingApproval(m.state.Tasks, id))
}

// priorityAfterApprove is what follows an approve: the priority picker, or nothing when a rating
// already releases the task. Clearing one of the two gates FEELS like releasing the work, which is
// how a dozen approved tasks came to sit inert, each owing a second keypress nobody knew about.
func (m model) priorityAfterApprove(id string) tea.Cmd {
	if api.ReleasedByPriority(m.state.Tasks)[id] {
		return nil
	}
	return func() tea.Msg { return openPriorityChoiceMsg(id) }
}

// taskPending reports whether a task is still held by the approval gate.
func (m model) taskPending(id string) bool {
	for _, t := range m.state.Tasks {
		if t.ID == id {
			return t.Approval == "pending"
		}
	}
	return false
}

// approveAfterPriority mirrors priorityAfterApprove: a rating landed on a task the gate still
// holds, so the other gate is OFFERED, never taken — approval decides what work exists, and
// granting it as a side effect would release work nobody agreed to.
func (m model) approveAfterPriority(id string) tea.Cmd {
	if !m.taskPending(id) {
		return nil
	}
	return func() tea.Msg { return openApproveAfterPriorityMsg(id) }
}

// openApproveAfterPriorityChoice offers the approve a rating does not perform: the claim queries
// want both gates, so a rated-but-unapproved task is as inert as an approved-but-unrated one.
// THIS TASK ONLY whatever the rating's scope — approving the children a scope reached would turn
// one confirm into a bulk verdict, so they are counted and named instead.
func (m *model) openApproveAfterPriorityChoice(id string) {
	cl := m.cl
	note := "Approving is what releases it to a worker; the priority alone does not.\n" +
		"Decline and it keeps the priority and stays in the backlog."
	if below := m.pendingBelow(id); below > 0 {
		note += "\n" + theme.Plural(below, "task", "tasks") + " below it also await approval; this does not touch them."
	}
	m.choice = choiceModalState{
		active: true, title: id + " is rated, but still awaits approval — approve it now?",
		note:    note,
		options: []string{"cancel", "approve " + id}, values: []string{"cancel", "approve"},
		apply: func(v string) tea.Cmd {
			if v != "approve" {
				return nil
			}
			// No follow-up: the rating that opened this modal is what priorityAfterApprove would
			// have asked for, so passing one would loop the pair of prompts back on itself.
			return taskOpTrigger(id, "approving", approveTaskCmd(cl, id, false, nil))
		},
	}
}

// openApproveChoice asks how far the verdict carries when the task has proposals under it. With
// nothing waiting below, keyApprove approves directly — there is nothing to choose between.
func (m *model) openApproveChoice(id string, pending int) {
	cl := m.cl
	then := m.priorityAfterApprove(id)
	kids := theme.Plural(pending, "subtask", "subtasks")
	m.choice = choiceModalState{
		active: true, title: "approve " + id + "?  (" + kids + " below await approval)",
		options: []string{"cancel", "approve this task only", "approve task + " + kids},
		values:  []string{"cancel", "task", "tree"},
		apply: func(v string) tea.Cmd {
			if v == "cancel" {
				return nil
			}
			subtree := v == "tree"
			return taskOpTrigger(id, "approving", approveTaskCmd(cl, id, subtree, then))
		},
	}
}
