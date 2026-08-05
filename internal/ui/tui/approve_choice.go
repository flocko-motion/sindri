// package: tui / approve choice
// type:    ui (Tasks tab confirm modal)
// job:     the approve confirm for a proposed task that has a tree under it — it counts the
// tasks below still awaiting a verdict and offers the wider approve, so a package reaches the
// backlog whole in one keypress.
// limits:  labels and key-to-call plumbing only; which tasks a verdict may reach is the hub's
// (-> api.PendingApproval) and the cascade itself is ApproveTask's.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/api"
)

// pendingBelow counts the tasks under id that still await the user's verdict.
func (m model) pendingBelow(id string) int {
	return len(api.PendingApproval(m.state.Tasks, id))
}

// openApproveChoice asks how far the verdict carries when the task has proposals under it. With
// nothing waiting below, keyApprove approves directly — there is nothing to choose between.
func (m *model) openApproveChoice(id string, pending int) {
	cl := m.cl
	kids := plural(pending, "subtask", "subtasks")
	m.choice = choiceModalState{
		active: true, title: "approve " + id + "?  (" + kids + " below await approval)",
		options: []string{"cancel", "approve this task only", "approve task + " + kids},
		values:  []string{"cancel", "task", "tree"},
		apply: func(v string) tea.Cmd {
			if v == "cancel" {
				return nil
			}
			subtree := v == "tree"
			return taskOpTrigger(id, "approving", approveTaskCmd(cl, id, subtree))
		},
	}
}
