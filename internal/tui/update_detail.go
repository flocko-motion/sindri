// package: tui / update_detail
// type:    ui
// job:     input handling for the detail view — every key dispatched while
//          the detail pane is open (comment, status, approve/merge/reject,
//          yank, scroll). Lives here so tui.go stays focused on Model + the
//          top-level Update.
// limits:  no rendering (-> detail.go), no action logic (-> action).
package tui

import (
	"fmt"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Sub-modes (comment, reject, status, confirm) are mutually exclusive
	// and each owns the whole input until dismissed; check them first so a
	// stray letter while typing doesn't trigger a detail-view hotkey.
	if m.commenting {
		return m.updateCommentInput(msg)
	}
	if m.rejecting {
		return m.updateRejectInput(msg)
	}
	if m.pickingStatus {
		return m.updateStatusPick(msg)
	}
	if m.confirmAction != "" {
		return m.updateConfirm(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewports()
		return m, nil
	case actionResultMsg:
		if msg.isError {
			m.notify = notification{message: msg.message, isError: true, time: time.Now()}
		} else {
			m.notify = notification{message: msg.message, time: time.Now()}
		}
		return m, tea.Batch(refreshAllCmd(m.projectRoot, false), flashTimer())
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "q"))):
			m.showDetail = false
			return m, nil
		case key.Matches(msg, keys.Up):
			m.vpDetail.LineUp(1)
		case key.Matches(msg, keys.Down):
			m.vpDetail.LineDown(1)
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+u"))):
			m.vpDetail.HalfViewUp()
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+d"))):
			m.vpDetail.HalfViewDown()
		case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
			m.vpDetail.GotoTop()
		case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
			m.vpDetail.GotoBottom()

		case key.Matches(msg, keys.EditTask):
			if m.detail.kind != detailTask || m.detail.taskID == "" {
				m.notify = notification{message: "Edit: only applies to tasks", isError: true, time: time.Now()}
				return m, flashTimer()
			}
			t := m.findTaskByID(m.detail.taskID)
			if t == nil {
				m.notify = notification{message: "Edit: task " + m.detail.taskID + " not found in board snapshot", isError: true, time: time.Now()}
				return m, flashTimer()
			}
			m.showCreateModal = true
			m.createModal = newEditTaskModel(m.projectRoot, *t)
			m.showDetail = false
			return m, m.createModal.Init()
		case key.Matches(msg, keys.Comment):
			if m.detail.kind == detailTask {
				ti := textinput.New()
				ti.Placeholder = "Type your comment..."
				ti.Focus()
				ti.CharLimit = 500
				ti.Width = m.width - 20
				m.commenting = true
				m.commentInput = ti
				return m, textinput.Blink
			}
		case key.Matches(msg, keys.Status):
			if m.detail.kind != detailTask {
				m.notify = notification{message: "Status: only applies to tasks", isError: true, time: time.Now()}
				return m, flashTimer()
			}
			m.openStatusPicker(m.detail.taskID, m.detail.taskStatus)
			return m, nil
		case key.Matches(msg, keys.Approve):
			if len(m.detail.prIDs) == 0 {
				// No PR → approve closes the task. See approveTaskNoPR.
				return m, m.approveTaskNoPR()
			}
			return m, m.approvePR()
		case key.Matches(msg, keys.Merge):
			if len(m.detail.prIDs) == 0 {
				m.notify = notification{message: "Merge: this task has no PR yet", isError: true, time: time.Now()}
				return m, flashTimer()
			}
			m.confirmAction = "merge"
			m.confirmLabel = fmt.Sprintf("Merge %s? (y/n)", m.detail.prIDs[0])
			return m, nil
		case key.Matches(msg, keys.Reject):
			if m.detail.kind == detailTask {
				ti := textinput.New()
				ti.Placeholder = "Reason for rejection..."
				ti.Focus()
				ti.CharLimit = 500
				ti.Width = m.width - 20
				m.rejecting = true
				m.commentInput = ti
				return m, textinput.Blink
			}
		case key.Matches(msg, keys.Yank):
			id := m.detail.taskID
			if id == "" && len(m.detail.prIDs) > 0 {
				id = m.detail.prIDs[0]
			}
			if id != "" {
				_ = clipboard.WriteAll(id)
				m.notify = notification{message: "Copied: " + id, time: time.Now()}
				return m, flashTimer()
			}
		}
	case notifyMsg:
		m.notify = notification{message: msg.message, isError: msg.isError, time: time.Now()}
		return m, flashTimer()
	case flashExpiredMsg:
		return m, nil
	}
	return m, nil
}
