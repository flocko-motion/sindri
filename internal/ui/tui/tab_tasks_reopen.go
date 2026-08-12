// package: tui / tasks_reopen
// type:    ui (Tasks tab — reopen a closed task)
// job:     the `O` action on the Tasks tab: gate it to closed tasks, and the reason
// form that restores one to open — the counterpart to close/openTaskRejectForm.
// limits:  form + gate only; the reopen itself, and the sindri-owned-only scope, are
// the hub's (-> client.ReopenTask -> workflow.Engine.ReopenTask).
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// taskReopenable reports whether the selected task is closed — the only state `O` acts on here. The
// hub, not this front-end, knows whether the id is actually sindri's to reopen (sd-/td- only).
func (m model) taskReopenable() bool {
	t, ok := m.selTask()
	return ok && t.Status == "closed"
}

// openTaskReopenForm restores a closed task to open, with a required reason recorded as a task
// comment — the counterpart to openTaskRejectForm, for the verdict going the other way. Validated,
// not just guarded on submit: closing on an empty reason would read as a successful reopen.
func (m *model) openTaskReopenForm(id string) {
	reason := newTextareaField("reason (recorded as a comment on the task)", "")
	validate := func() string {
		if strings.TrimSpace(reason.value()) == "" {
			return "say why — the reason is recorded as a comment on the task"
		}
		return ""
	}
	cl := m.cl
	m.form.open("reopen task "+id, []field{reason}, validate, func() tea.Cmd {
		text := reason.value()
		return func() tea.Msg {
			if cl == nil {
				return nil
			}
			if err := cl.ReopenTask(id, text); err != nil {
				return errModalMsg{err}
			}
			st, _ := cl.State()
			return polledMsg(st)
		}
	})
}
