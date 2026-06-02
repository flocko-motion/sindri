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
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/flo-at/sindri/internal/action"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/board"
	"github.com/flo-at/sindri/internal/issue"
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

// statusOptions returns the canonical td status list shown by the picker.
func statusOptions() []string {
	return []string{"open", "in_progress", "in_review", "blocked", "closed"}
}

// statusAtCursor returns the (taskID, status) of the task at the backlog
// cursor; empty strings when the cursor is not on a task row (spec-only Issue,
// PR sub-row, or a different left-view).
func (m Model) statusAtCursor() (taskID, status string) {
	if m.leftView != viewBacklog {
		return "", ""
	}
	if m.listCursor < 0 || m.listCursor >= len(m.backlogRows) {
		return "", ""
	}
	row := m.backlogRows[m.listCursor]
	if row.isPR || row.issueIdx < 0 || row.issueIdx >= len(m.visibleIssues) {
		return "", ""
	}
	iss := m.visibleIssues[row.issueIdx]
	if iss.Task == nil {
		return "", ""
	}
	return iss.Task.ID, iss.Task.Status
}

// openStatusPicker primes the status picker for the given task. setTaskStatus
// reads m.detail.taskID, so we stash the id there too — that way the picker
// works whether opened from the list view or the detail view.
func (m *Model) openStatusPicker(taskID, currentStatus string) {
	m.detail.taskID = taskID
	m.detail.taskStatus = currentStatus
	m.statusOptions = statusOptions()
	m.statusCursor = 0
	for i, opt := range m.statusOptions {
		if opt == currentStatus {
			m.statusCursor = i
		}
	}
	m.pickingStatus = true
}

// renderStatusPicker draws the picker as a single-line pill row: the cursor
// option is wrapped in brackets, the rest are separated by middle dots.
func renderStatusPicker(opts []string, cursor int) string {
	parts := make([]string, len(opts))
	for i, opt := range opts {
		if i == cursor {
			parts[i] = "[" + opt + "]"
		} else {
			parts[i] = opt
		}
	}
	return "Status: " + strings.Join(parts, " · ") + "   (←/→ pick, enter apply, esc cancel)"
}

// updateStatusPick handles input while the status picker is open.
func (m Model) updateStatusPick(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.pickingStatus = false
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			if m.statusCursor > 0 {
				m.statusCursor--
			}
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			if m.statusCursor < len(m.statusOptions)-1 {
				m.statusCursor++
			}
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			chosen := m.statusOptions[m.statusCursor]
			m.pickingStatus = false
			return m, m.setTaskStatus(chosen)
		}
	}
	return m, nil
}

// taskAtCursor returns the task at the backlog cursor and its parent_id;
// empty strings if the cursor isn't on a task row (spec-only, PR sub-row, or
// the workers panel).
func (m Model) taskAtCursor() (taskID, parentID string) {
	if m.leftView != viewBacklog || m.listCursor < 0 || m.listCursor >= len(m.backlogRows) {
		return "", ""
	}
	row := m.backlogRows[m.listCursor]
	if row.isPR || row.issueIdx < 0 || row.issueIdx >= len(m.visibleIssues) {
		return "", ""
	}
	iss := m.visibleIssues[row.issueIdx]
	if iss.Task == nil {
		return "", ""
	}
	return iss.Task.ID, iss.Task.ParentID
}

// enterMoveMode marks the task at cursor as "in movement" and shows the user
// the next-step hint. A non-task cursor produces a visible error rather than
// silently doing nothing.
func (m Model) enterMoveMode() (tea.Model, tea.Cmd) {
	taskID, _ := m.taskAtCursor()
	if taskID == "" {
		m.notify = notification{message: "Move: pick a task row first", isError: true, time: time.Now()}
		return m, flashTimer()
	}
	m.moving = true
	m.movingTaskID = taskID
	m.rebuildBacklog()
	m.notify = notification{message: "Moving " + taskID + " — h: sibling of cursor, l: child of cursor, esc: cancel", time: time.Now()}
	return m, flashTimer()
}

// cancelMove clears move mode with no state change.
func (m *Model) cancelMove() {
	m.moving = false
	m.movingTaskID = ""
	m.rebuildBacklog()
}

// applyMove commits the pending move. asChild=true ⇒ the moving task becomes
// a child of the target row (parent_id = target.id); asChild=false ⇒ a sibling
// of the target row (parent_id = target.parent_id). Refuses self-target and
// cycles (target = source, or target descends from source) with a visible
// notification and keeps move mode active.
func (m Model) applyMove(asChild bool) (tea.Model, tea.Cmd) {
	src := m.movingTaskID
	targetID, targetParent := m.taskAtCursor()
	if src == "" || targetID == "" {
		m.notify = notification{message: "Move: cursor must be on a task", isError: true, time: time.Now()}
		return m, flashTimer()
	}
	if targetID == src {
		m.notify = notification{message: "Move: target must be a different task", isError: true, time: time.Now()}
		return m, flashTimer()
	}
	newParent := targetParent
	if asChild {
		newParent = targetID
	}
	if newParent == src {
		// Asking to make src a child of src — same prohibition as self-target.
		m.notify = notification{message: "Move: cannot make a task its own parent", isError: true, time: time.Now()}
		return m, flashTimer()
	}
	if descendantOfSource(m.issues, src, newParent) {
		m.notify = notification{message: "Move: target is a descendant of " + src + " — would create a cycle", isError: true, time: time.Now()}
		return m, flashTimer()
	}
	// Commit: clear move state, fire the update, refresh on success.
	m.moving = false
	m.movingTaskID = ""
	return m, m.setTaskParent(src, newParent)
}

// descendantOfSource reports whether candidate is src or any of src's
// descendants in the current issue set.
func descendantOfSource(issues []issue.Issue, src, candidate string) bool {
	if src == "" || candidate == "" {
		return false
	}
	parentOf := map[string]string{}
	for _, iss := range issues {
		if iss.Task != nil {
			parentOf[iss.Task.ID] = iss.Task.ParentID
		}
	}
	for cur := candidate; cur != ""; cur = parentOf[cur] {
		if cur == src {
			return true
		}
	}
	return false
}

// setTaskParent re-parents a task via the td adapter and triggers a manual
// refresh so the new hierarchy redraws. The parent_id cache is updated in
// place so the refresh sees the new structure without a second WarmParentCache.
func (m *Model) setTaskParent(taskID, newParent string) tea.Cmd {
	projectRoot := m.projectRoot
	return func() tea.Msg {
		if err := td.SetParent(projectRoot, taskID, newParent); err != nil {
			return actionResultMsg{message: "Move failed: " + err.Error(), isError: true}
		}
		board.SetCachedParent(taskID, newParent)
		return actionResultMsg{message: "Moved " + taskID}
	}
}

// setTaskStatus applies the chosen status through the td adapter.
func (m *Model) setTaskStatus(status string) tea.Cmd {
	taskID := m.detail.taskID
	projectRoot := m.projectRoot
	prev := m.detail.taskStatus
	return func() tea.Msg {
		if err := td.SetStatus(projectRoot, taskID, status); err != nil {
			return actionResultMsg{message: "Status change failed: " + err.Error(), isError: true}
		}
		return actionResultMsg{message: fmt.Sprintf("Status: %s → %s", prev, status)}
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
