// package: tui / actions
// type:    ui
// job:     detail-view actions — comment + reject-reason input, the merge
//          confirmation gate, and the commands that approve/merge/reject/
//          comment/cycle a task. Each calls the shared action layer (or the td
//          adapter) and reports an actionResultMsg.
// limits:  no domain rules (-> issue/action); mutations go through
//          internal/action and adapter/td, never raw exec.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/flo-at/sindri/internal/action"
	"github.com/flo-at/sindri/internal/adapter/td"
)

type actionResultMsg struct {
	message string
	isError bool
}

func (m Model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			act := m.confirmAction
			m.confirmAction = ""
			m.confirmLabel = ""
			if act == "merge" {
				return m, m.mergePR()
			}
		default:
			m.confirmAction = ""
			m.confirmLabel = ""
		}
	}
	return m, nil
}

func (m Model) updateCommentInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.commenting = false
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			text := strings.TrimSpace(m.commentInput.Value())
			if text == "" {
				m.commenting = false
				return m, nil
			}
			m.commenting = false
			return m, m.addComment(text)
		}
	}
	var cmd tea.Cmd
	m.commentInput, cmd = m.commentInput.Update(msg)
	return m, cmd
}

// updateRejectInput collects the (required) rejection reason, then rejects.
func (m Model) updateRejectInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.rejecting = false
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			text := strings.TrimSpace(m.commentInput.Value())
			m.rejecting = false
			if text == "" {
				return m, nil // no reason → cancel; reject always needs one
			}
			return m, m.rejectTask(text)
		}
	}
	var cmd tea.Cmd
	m.commentInput, cmd = m.commentInput.Update(msg)
	return m, cmd
}

func (m *Model) approvePR() tea.Cmd {
	prID := m.detail.prIDs[0]
	root := m.projectRoot
	return func() tea.Msg {
		pr, err := action.Approve(root, prID)
		if err != nil {
			return actionResultMsg{message: "Approve failed: " + err.Error(), isError: true}
		}
		return actionResultMsg{message: "PR approved: " + pr.ID}
	}
}

func (m *Model) mergePR() tea.Cmd {
	prID := m.detail.prIDs[0]
	root := m.projectRoot
	return func() tea.Msg {
		merged, missing, err := action.Merge(root, prID)
		if err != nil {
			return actionResultMsg{message: "Merge failed: " + err.Error(), isError: true}
		}
		if len(missing) > 0 {
			return actionResultMsg{message: "Merge blocked — unmet gates: " + strings.Join(missing, ", "), isError: true}
		}
		return actionResultMsg{message: "PR merged: " + merged.ID}
	}
}

func (m *Model) rejectTask(reason string) tea.Cmd {
	prID := ""
	if len(m.detail.prIDs) > 0 {
		prID = m.detail.prIDs[0]
	}
	taskID := m.detail.taskID
	root := m.projectRoot
	return func() tea.Msg {
		if prID != "" {
			pr, err := action.Reject(root, prID, reason)
			if err != nil {
				return actionResultMsg{message: "Reject failed: " + err.Error(), isError: true}
			}
			return actionResultMsg{message: "Rejected PR " + pr.ID + " and reopened its task"}
		}
		if err := action.RejectTask(root, taskID, reason); err != nil {
			return actionResultMsg{message: "Reject failed: " + err.Error(), isError: true}
		}
		return actionResultMsg{message: "Rejected task " + taskID}
	}
}

func (m *Model) cycleTaskStatus() tea.Cmd {
	taskID := m.detail.taskID
	projectRoot := m.projectRoot
	return func() tea.Msg {
		t, err := td.Get(projectRoot, taskID)
		if err != nil {
			return actionResultMsg{message: "Failed to read task", isError: true}
		}
		var next string
		switch t.Status {
		case "open":
			next = "in_progress"
		case "in_progress":
			next = "open"
		default:
			return actionResultMsg{message: "Cannot change status from " + t.Status, isError: true}
		}
		if err := td.SetStatus(projectRoot, taskID, next); err != nil {
			return actionResultMsg{message: "Status change failed: " + err.Error(), isError: true}
		}
		return actionResultMsg{message: fmt.Sprintf("Status: %s → %s", t.Status, next)}
	}
}

func (m *Model) addComment(text string) tea.Cmd {
	taskID := m.detail.taskID
	projectRoot := m.projectRoot
	return func() tea.Msg {
		if err := td.Comment(projectRoot, taskID, text); err != nil {
			return actionResultMsg{message: "Comment failed: " + err.Error(), isError: true}
		}
		return actionResultMsg{message: "Comment added"}
	}
}
