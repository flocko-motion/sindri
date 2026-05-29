// package: tui / tui
// type:    ui
// job:     the Bubble Tea root model — backlog/workers views, navigation,
//          detail drill-down, and the refresh cycle over board.List state.
// limits:  thin renderer — data via board/issue, styling via render, mutations
//          via the action commands in actions.go (-> adapter/td, ghlocal/store).
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/issue"
	"github.com/flo-at/sindri/internal/worker"
)

const (
	viewBacklog = iota
	viewWorkers
)

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
	issues  []issue.Issue
	detail  detailState

	vpList   viewport.Model
	vpDetail viewport.Model

	showDetail      bool
	showCreateModal bool
	createModal     createTaskModel

	confirmAction string
	confirmLabel  string

	commenting   bool
	rejecting    bool
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
		m.issues = msg.issues
		m.rebuildBacklog()
		m.clampCursors()
		m.syncListScroll()
		if m.showDetail && m.detail.taskID != "" {
			for _, iss := range m.issues {
				if iss.ID() == m.detail.taskID {
					m.detail = issueDetail(iss, m.projectRoot)
					m.vpDetail.SetContent(m.detail.content)
					break
				}
			}
		}
		if msg.manual {
			m.notify = notification{
				message: fmt.Sprintf("Refreshed — %d items, %d workers", len(m.issues), len(m.workers)),
				time:    time.Now(),
			}
			return m, flashTimer()
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
			m.syncListScroll()
		case key.Matches(msg, keys.Workers):
			m.leftView = viewWorkers
			m.syncListScroll()
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
			return m, refreshDataManual(m.projectRoot)
		}

	case notifyMsg:
		m.notify = notification{message: msg.message, isError: msg.isError, time: time.Now()}
		return m, flashTimer()

	case flashExpiredMsg:
		return m, nil
	}

	return m, nil
}

func (m Model) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle comment input mode
	if m.commenting {
		return m.updateCommentInput(msg)
	}

	// Handle reject-reason input mode
	if m.rejecting {
		return m.updateRejectInput(msg)
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
	m.syncListScroll()
}

func (m *Model) rebuildBacklog() {
	m.backlogRows = buildBacklogRows(m.issues)
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
				m.syncListScroll()
				return
			}
		}
	case viewWorkers:
		next := m.workerCursor + delta
		if next >= 0 && next < len(m.workers) {
			m.workerCursor = next
			m.syncListScroll()
		}
	}
}

// syncListScroll adjusts the left-column viewport offset so the active cursor
// stays visible, scrolling only when the cursor moves outside the window. It is
// driven from Update (not View) so the offset persists across frames.
func (m *Model) syncListScroll() {
	h := m.vpList.Height
	if h < 1 {
		return
	}
	var cursor, total int
	switch m.leftView {
	case viewBacklog:
		cursor, total = m.listCursor, len(m.backlogRows)
	case viewWorkers:
		cursor, total = m.workerCursor, len(m.workers)
	}
	off := m.vpList.YOffset
	if cursor < off {
		off = cursor
	} else if cursor >= off+h {
		off = cursor - h + 1
	}
	if maxOff := total - h; off > maxOff {
		off = maxOff
	}
	if off < 0 {
		off = 0
	}
	m.vpList.YOffset = off
}

func (m *Model) selectedID() string {
	switch m.leftView {
	case viewBacklog:
		if m.listCursor < len(m.backlogRows) {
			row := m.backlogRows[m.listCursor]
			if row.isPR {
				return row.pr.ID
			}
			if row.issueIdx < len(m.issues) {
				return m.issues[row.issueIdx].ID()
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
		m.syncListScroll()
	}
}

func (m *Model) navigateOut() {
	for pos := m.listCursor - 1; pos >= 0; pos-- {
		if !m.backlogRows[pos].isPR {
			m.listCursor = pos
			m.syncListScroll()
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
				m.detail = prDetail(row.pr)
			} else if row.issueIdx < len(m.issues) {
				m.detail = issueDetail(m.issues[row.issueIdx], m.projectRoot)
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
	oldPRs := map[string]string{}
	oldTasks := map[string]string{}
	for _, iss := range m.issues {
		if iss.Task != nil {
			oldTasks[iss.Task.ID] = iss.Task.Status
		}
		for _, pr := range iss.PRs {
			oldPRs[pr.ID] = pr.Status
		}
	}
	for _, iss := range msg.issues {
		for _, pr := range iss.PRs {
			old, existed := oldPRs[pr.ID]
			if !existed {
				m.notify = notification{message: fmt.Sprintf("PR created: %s", pr.ID), time: time.Now()}
			} else if old != pr.Status {
				m.notify = notification{message: fmt.Sprintf("PR %s: %s → %s", pr.ID, old, pr.Status), time: time.Now()}
			}
		}
		if iss.Task != nil {
			if old, existed := oldTasks[iss.Task.ID]; existed && old != iss.Task.Status {
				m.notify = notification{message: fmt.Sprintf("Task %s: %s → %s", iss.Task.ID, old, iss.Task.Status), time: time.Now()}
			}
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
	switch m.leftView {
	case viewBacklog:
		header = "Tasks"
		listContent = renderBacklogList(m.backlogRows, m.listCursor, true)
	case viewWorkers:
		header = "Workers"
		listContent = renderWorkersList(m.workers, m.workerCursor, true)
	}
	m.vpList.SetContent(strings.TrimRight(listContent, "\n"))

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
	} else if m.rejecting {
		bottomBar = lipgloss.NewStyle().PaddingLeft(1).Render("Reject reason: " + m.commentInput.View())
	} else {
		bottomBar = m.notify.render(m.width)
	}
	return titleBar + "\n" + col + "\n" + bottomBar
}

func renderColumn(header, content, footer string, width, height int, active bool) string {
	// lipgloss Width/Height size the content box; the border is drawn outside
	// it. Subtract the left+right border so the column fits exactly in `width`,
	// otherwise the right border overflows the terminal and is clipped.
	style := columnStyle.Width(width - 2).Height(height)
	if active {
		style = activeColumnStyle.Width(width - 2).Height(height)
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
