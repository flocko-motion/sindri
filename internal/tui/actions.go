// package: tui / actions
// type:    ui
// job:     detail-view actions — comment input, the merge/reject confirmation
//          gate, and the commands that approve/merge/reject/comment/cycle a
//          task. Each command calls the logic layer and reports an actionResultMsg.
// limits:  no domain rules (-> issue); mutations go through adapter/td and
//          ghlocal/store, never raw exec.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/ghlocal/store"
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
			action := m.confirmAction
			m.confirmAction = ""
			m.confirmLabel = ""
			switch action {
			case "merge":
				return m, m.mergePR()
			case "reject":
				return m, m.rejectTask()
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

func (m *Model) approvePR() tea.Cmd {
	prID := m.detail.prIDs[0]
	return func() tea.Msg {
		_, err := store.Approve(prID)
		if err != nil {
			return actionResultMsg{message: "Approve failed: " + err.Error(), isError: true}
		}
		return actionResultMsg{message: "PR approved: " + prID}
	}
}

func (m *Model) mergePR() tea.Cmd {
	prID := m.detail.prIDs[0]
	return func() tea.Msg {
		_, err := store.Merge(prID)
		if err != nil {
			return actionResultMsg{message: "Merge failed: " + err.Error(), isError: true}
		}
		return actionResultMsg{message: "PR merged: " + prID}
	}
}

func (m *Model) rejectTask() tea.Cmd {
	taskID := m.detail.taskID
	projectRoot := m.projectRoot
	prIDs := make([]string, len(m.detail.prIDs))
	copy(prIDs, m.detail.prIDs)
	return func() tea.Msg {
		if err := td.Reject(projectRoot, taskID); err != nil {
			return actionResultMsg{message: "Reject failed: " + err.Error(), isError: true}
		}
		for _, prID := range prIDs {
			pr, err := store.Read(prID)
			if err != nil {
				continue
			}
			if pr.Status == "open" || pr.Status == "approved" {
				pr.Status = "rejected"
				if writeErr := store.Write(pr); writeErr != nil {
					return actionResultMsg{message: "Task rejected but PR update failed: " + writeErr.Error(), isError: true}
				}
			}
		}
		return actionResultMsg{message: "Task rejected: " + taskID}
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
