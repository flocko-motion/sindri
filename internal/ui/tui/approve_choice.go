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

// priorityAfterApprove is what follows an approve: the priority picker for the task just approved,
// or nothing when a rating already releases it.
//
// Approving and rating are two gates, and clearing one FEELS like releasing the work — which is how a
// dozen approved tasks came to sit in the backlog inert, each waiting on a second keypress nobody knew
// was owed. So the second gate is offered rather than remembered. A task already released by its own
// priority or an ancestor's needs nothing, and asking anyway would be the friction in reverse.
func (m model) priorityAfterApprove(id string) tea.Cmd {
	if api.ReleasedByPriority(m.state.Tasks)[id] {
		return nil
	}
	return func() tea.Msg { return openPriorityChoiceMsg(id) }
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
