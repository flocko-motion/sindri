package tui

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/flo-at/sindri/internal/worker"
)

const (
	viewBacklog = iota
	viewWorkers
)

var taskIDRe = regexp.MustCompile(`\(?(td-[0-9a-f]+)\)?`)

const refreshInterval = 5 * time.Second

type Model struct {
	projectRoot string
	width       int
	height      int

	leftView int // viewBacklog or viewWorkers

	listCursor   int
	backlogRows  []backlogRow
	workerCursor int

	workers []worker.Worker
	tasks   []taskItem
	prs     []prItem
	detail  detailState

	vpList   viewport.Model
	vpDetail viewport.Model

	showDetail      bool
	showCreateModal bool
	createModal     createTaskModel

	confirmAction string
	confirmLabel  string

	commenting   bool
	commentInput textinput.Model

	notify notification
}

func New(projectRoot string) Model {
	return Model{
		projectRoot: projectRoot,
		leftView:    viewBacklog,
		vpList:      viewport.New(0, 0),
		vpDetail:    viewport.New(0, 0),
	}
}

type tickMsg struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refreshData(m.projectRoot), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		return m, tea.Batch(refreshData(m.projectRoot), tickCmd())
	case refreshMsg:
		m.detectChanges(msg)
		m.workers = msg.workers
		m.tasks = msg.tasks
		m.prs = msg.prs
		m.rebuildBacklog()
		m.clampCursors()
		if m.showDetail && m.detail.taskID != "" {
			for _, t := range m.tasks {
				if t.ID == m.detail.taskID {
					m.detail = taskDetail(t, m.prs, m.workers, m.projectRoot)
					m.vpDetail.SetContent(m.detail.content)
					break
				}
			}
		}
		return m, nil
	}

	if m.showCreateModal {
		return m.updateModal(msg)
	}

	if m.showDetail {
		return m.updateDetail(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewports()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.NewTask):
			m.showCreateModal = true
			m.createModal = newCreateTaskModel(m.projectRoot)
			return m, m.createModal.Init()
		case key.Matches(msg, keys.Backlog):
			m.leftView = viewBacklog
			m.rebuildBacklog()
		case key.Matches(msg, keys.Workers):
			m.leftView = viewWorkers
		case key.Matches(msg, keys.Up):
			m.moveCursor(-1)
		case key.Matches(msg, keys.Down):
			m.moveCursor(1)
		case key.Matches(msg, key.NewBinding(key.WithKeys("l"))):
			if m.leftView == viewBacklog {
				m.navigateInto()
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("h"))):
			if m.leftView == viewBacklog {
				m.navigateOut()
			}
		case key.Matches(msg, keys.Enter):
			m.openDetail()
		case key.Matches(msg, keys.Yank):
			if id := m.selectedID(); id != "" {
				_ = clipboard.WriteAll(id)
				m.notify = notification{message: "Copied: " + id, time: time.Now()}
				return m, flashTimer()
			}
		case key.Matches(msg, keys.Refresh):
			return m, refreshData(m.projectRoot)
		}

	case notifyMsg:
		m.notify = notification{message: msg.message, isError: msg.isError, time: time.Now()}
		return m, flashTimer()

	case flashExpiredMsg:
		return m, nil
	}

	return m, nil
}

type actionResultMsg struct {
	message string
	isError bool
}

func (m Model) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle comment input mode
	if m.commenting {
		return m.updateCommentInput(msg)
	}

	// Handle confirmation mode
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
		return m, tea.Batch(refreshData(m.projectRoot), flashTimer())
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

		// Action hotkeys (only for task detail)
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
			if m.detail.kind == detailTask {
				return m, m.cycleTaskStatus()
			}
		case key.Matches(msg, keys.Approve):
			if len(m.detail.prIDs) > 0 {
				return m, m.approvePR()
			}
		case key.Matches(msg, keys.Merge):
			if len(m.detail.prIDs) > 0 {
				m.confirmAction = "merge"
				m.confirmLabel = fmt.Sprintf("Merge %s? (y/n)", m.detail.prIDs[0])
				return m, nil
			}
		case key.Matches(msg, keys.Reject):
			if m.detail.kind == detailTask {
				m.confirmAction = "reject"
				m.confirmLabel = fmt.Sprintf("Reject task %s? (y/n)", m.detail.taskID)
				return m, nil
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
		out, err := exec.Command("td", "-w", projectRoot, "reject", taskID).CombinedOutput()
		if err != nil {
			return actionResultMsg{message: fmt.Sprintf("Reject failed: %s", strings.TrimSpace(string(out))), isError: true}
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
		out, err := exec.Command("td", "-w", projectRoot, "show", taskID, "--json").Output()
		if err != nil {
			return actionResultMsg{message: "Failed to read task", isError: true}
		}
		var current struct {
			Status string `json:"status"`
		}
		if jsonErr := json.Unmarshal(out, &current); jsonErr != nil {
			return actionResultMsg{message: "Failed to parse task", isError: true}
		}
		var next string
		switch current.Status {
		case "open":
			next = "in_progress"
		case "in_progress":
			next = "open"
		default:
			return actionResultMsg{message: "Cannot change status from " + current.Status, isError: true}
		}
		out, err = exec.Command("td", "-w", projectRoot, "update", taskID, "--status", next).CombinedOutput()
		if err != nil {
			return actionResultMsg{message: fmt.Sprintf("Status change failed: %s", strings.TrimSpace(string(out))), isError: true}
		}
		return actionResultMsg{message: fmt.Sprintf("Status: %s → %s", current.Status, next)}
	}
}

func (m *Model) addComment(text string) tea.Cmd {
	taskID := m.detail.taskID
	projectRoot := m.projectRoot
	return func() tea.Msg {
		out, err := exec.Command("td", "-w", projectRoot, "comment", taskID, text).CombinedOutput()
		if err != nil {
			return actionResultMsg{message: fmt.Sprintf("Comment failed: %s", strings.TrimSpace(string(out))), isError: true}
		}
		return actionResultMsg{message: "Comment added"}
	}
}

func (m *Model) resizeViewports() {
	contentHeight := m.height - 4
	innerH := contentHeight - 4 // border(2) + header(2)
	if innerH < 1 {
		innerH = 1
	}
	m.vpList.Width = m.width - 4
	m.vpList.Height = innerH
	m.vpDetail.Width = m.width - 4
	m.vpDetail.Height = innerH - 1
	if m.vpDetail.Height < 1 {
		m.vpDetail.Height = 1
	}
}

func (m *Model) rebuildBacklog() {
	workersByTask := make(map[string]string)
	for _, wk := range m.workers {
		if wk.Task != "" {
			parts := strings.Fields(wk.Task)
			if len(parts) > 0 {
				workersByTask[parts[0]] = wk.Name
			}
		}
	}
	m.backlogRows = buildBacklogRows(m.tasks, m.prs, workersByTask)
}

func (m *Model) moveCursor(delta int) {
	switch m.leftView {
	case viewBacklog:
		pos := m.listCursor
		for {
			pos += delta
			if pos < 0 || pos >= len(m.backlogRows) {
				return
			}
			if !m.backlogRows[pos].isPR {
				m.listCursor = pos
				return
			}
		}
	case viewWorkers:
		next := m.workerCursor + delta
		if next >= 0 && next < len(m.workers) {
			m.workerCursor = next
		}
	}
}

func (m *Model) selectedID() string {
	switch m.leftView {
	case viewBacklog:
		if m.listCursor < len(m.backlogRows) {
			row := m.backlogRows[m.listCursor]
			if row.isPR {
				if row.prIdx < len(m.prs) {
					return m.prs[row.prIdx].ID
				}
			} else {
				if row.taskIdx < len(m.tasks) {
					return m.tasks[row.taskIdx].ID
				}
			}
		}
	case viewWorkers:
		if m.workerCursor < len(m.workers) {
			return m.workers[m.workerCursor].Name
		}
	}
	return ""
}

func (m *Model) navigateInto() {
	if m.listCursor+1 < len(m.backlogRows) && m.backlogRows[m.listCursor+1].isPR {
		m.listCursor++
	}
}

func (m *Model) navigateOut() {
	for pos := m.listCursor - 1; pos >= 0; pos-- {
		if !m.backlogRows[pos].isPR {
			m.listCursor = pos
			return
		}
	}
}

func (m *Model) openDetail() {
	switch m.leftView {
	case viewBacklog:
		if m.listCursor < len(m.backlogRows) {
			row := m.backlogRows[m.listCursor]
			if row.isPR {
				if row.prIdx < len(m.prs) {
					m.detail = prDetail(m.prs[row.prIdx])
				}
			} else {
				if row.taskIdx < len(m.tasks) {
					m.detail = taskDetail(m.tasks[row.taskIdx], m.prs, m.workers, m.projectRoot)
				}
			}
		}
	case viewWorkers:
		if m.workerCursor < len(m.workers) {
			m.detail = workerDetail(m.workers[m.workerCursor])
		}
	}
	m.vpDetail.SetContent(m.detail.content)
	m.vpDetail.GotoTop()
	m.showDetail = true
}

func (m *Model) detectChanges(msg refreshMsg) {
	oldPRs := make(map[string]string)
	for _, pr := range m.prs {
		oldPRs[pr.ID] = pr.Status
	}
	for _, pr := range msg.prs {
		old, existed := oldPRs[pr.ID]
		if !existed {
			m.notify = notification{message: fmt.Sprintf("PR created: %s", pr.ID), time: time.Now()}
		} else if old != pr.Status {
			m.notify = notification{message: fmt.Sprintf("PR %s: %s → %s", pr.ID, old, pr.Status), time: time.Now()}
		}
	}

	oldTasks := make(map[string]string)
	for _, t := range m.tasks {
		oldTasks[t.ID] = t.Status
	}
	for _, t := range msg.tasks {
		old, existed := oldTasks[t.ID]
		if existed && old != t.Status {
			m.notify = notification{message: fmt.Sprintf("Task %s: %s → %s", t.ID, old, t.Status), time: time.Now()}
		}
	}
}

func (m *Model) clampCursors() {
	if n := len(m.backlogRows); m.listCursor >= n && n > 0 {
		m.listCursor = n - 1
	}
	if n := len(m.workers); m.workerCursor >= n && n > 0 {
		m.workerCursor = n - 1
	}
}

func (m Model) updateModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		if key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
			m.showCreateModal = false
			return m, nil
		}
	case taskCreatedMsg:
		m.showCreateModal = false
		if msg.err != nil {
			m.notify = notification{message: "Error: " + msg.err.Error(), isError: true, time: time.Now()}
			return m, flashTimer()
		}
		m.notify = notification{message: "Task created: " + msg.id, time: time.Now()}
		return m, tea.Batch(refreshData(m.projectRoot), flashTimer())
	}
	var cmd tea.Cmd
	m.createModal, cmd = m.createModal.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}
	if m.showCreateModal {
		return m.createModal.View(m.width, m.height)
	}
	if m.showDetail {
		return m.viewDetail()
	}
	return m.viewList()
}

func (m Model) viewList() string {
	title := titleStyle.Render("Sindri — AI Agent Orchestrator")

	activeView := lipgloss.NewStyle().Bold(true).Foreground(highlight)
	inactiveView := dimStyle
	var viewSelector string
	if m.leftView == viewBacklog {
		viewSelector = activeView.Render("[T]asks") + "  " + inactiveView.Render("[W]orkers")
	} else {
		viewSelector = inactiveView.Render("[T]asks") + "  " + activeView.Render("[W]orkers")
	}
	help := dimStyle.Render("j/k:nav  enter:open  y:copy  n:new  r:refresh  q:quit")
	rightSide := viewSelector + "  " + help

	titleBar := lipgloss.JoinHorizontal(lipgloss.Top,
		title,
		lipgloss.NewStyle().Width(m.width-lipgloss.Width(title)-lipgloss.Width(rightSide)).Render(""),
		rightSide,
	)

	contentHeight := m.height - 4

	var listContent string
	var header string
	var cursorLine int
	switch m.leftView {
	case viewBacklog:
		header = "Tasks"
		listContent = renderBacklogList(m.backlogRows, m.listCursor, true)
		cursorLine = m.listCursor
	case viewWorkers:
		header = "Workers"
		listContent = renderWorkersList(m.workers, m.workerCursor, true)
		cursorLine = m.workerCursor
	}
	m.vpList.SetContent(listContent)

	if cursorLine < m.vpList.YOffset {
		m.vpList.SetYOffset(cursorLine)
	} else if cursorLine >= m.vpList.YOffset+m.vpList.Height {
		m.vpList.SetYOffset(cursorLine - m.vpList.Height + 1)
	}

	scrollStatus := ""
	if m.vpList.TotalLineCount() > m.vpList.Height {
		pct := int(m.vpList.ScrollPercent() * 100)
		scrollStatus = dimStyle.Render(fmt.Sprintf(" %d%% (%d/%d)", pct, m.vpList.YOffset+m.vpList.Height, m.vpList.TotalLineCount()))
	}

	col := renderColumn(header, m.vpList.View(), scrollStatus, m.width, contentHeight, true)

	notifyBar := m.notify.render(m.width)
	return titleBar + "\n" + col + "\n" + notifyBar
}

func (m Model) viewDetail() string {
	title := titleStyle.Render(m.detail.title)
	var help string
	if m.detail.kind == detailTask {
		help = dimStyle.Render("j/k:scroll  c:comment  s:status  a:approve  m:merge  x:reject  y:copy  esc:back")
	} else {
		help = dimStyle.Render("j/k:scroll  a:approve  m:merge  y:copy  esc:back")
	}
	titleBar := lipgloss.JoinHorizontal(lipgloss.Top,
		title,
		lipgloss.NewStyle().Width(m.width-lipgloss.Width(title)-lipgloss.Width(help)).Render(""),
		help,
	)

	contentHeight := m.height - 4

	scrollStatus := ""
	if m.vpDetail.TotalLineCount() > m.vpDetail.Height {
		pct := int(m.vpDetail.ScrollPercent() * 100)
		scrollStatus = dimStyle.Render(fmt.Sprintf(" %d%% (%d/%d)", pct, m.vpDetail.YOffset+m.vpDetail.Height, m.vpDetail.TotalLineCount()))
	}

	col := renderColumn("", m.vpDetail.View(), scrollStatus, m.width, contentHeight, true)

	var bottomBar string
	if m.confirmAction != "" {
		confirmStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#FF6600")).
			PaddingLeft(1).
			PaddingRight(1)
		bottomBar = confirmStyle.Width(m.width).Render(m.confirmLabel)
	} else if m.commenting {
		bottomBar = lipgloss.NewStyle().PaddingLeft(1).Render("Comment: " + m.commentInput.View())
	} else {
		bottomBar = m.notify.render(m.width)
	}
	return titleBar + "\n" + col + "\n" + bottomBar
}

func renderColumn(header, content, footer string, width, height int, active bool) string {
	style := columnStyle.Width(width).Height(height)
	if active {
		style = activeColumnStyle.Width(width).Height(height)
	}
	var full string
	if header != "" {
		full = headerStyle.Render(header) + "\n" + content
	} else {
		full = content
	}
	if footer != "" {
		full += "\n" + footer
	}
	return style.Render(full)
}
