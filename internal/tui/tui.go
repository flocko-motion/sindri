package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
		case key.Matches(msg, keys.Refresh):
			return m, refreshData(m.projectRoot)
		}

	case notifyMsg:
		m.notify = notification{message: msg.message, isError: msg.isError, time: time.Now()}
		return m, flashTimer()

	case flashExpiredMsg:
		return m, nil

	case refreshMsg:
		m.detectChanges(msg)
		m.workers = msg.workers
		m.tasks = msg.tasks
		m.prs = msg.prs
		m.rebuildBacklog()
		m.clampCursors()

	case tickMsg:
		return m, tea.Batch(refreshData(m.projectRoot), tickCmd())
	}

	return m, nil
}

func (m Model) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewports()
		return m, nil
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
		}
	case refreshMsg:
		m.detectChanges(msg)
		m.workers = msg.workers
		m.tasks = msg.tasks
		m.prs = msg.prs
		m.rebuildBacklog()
		m.clampCursors()
	case tickMsg:
		return m, tea.Batch(refreshData(m.projectRoot), tickCmd())
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
	help := dimStyle.Render("j/k:nav  enter:open  n:new  r:refresh  q:quit")
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
	m.vpList.SetContent(listContent)

	col := renderColumn(header, m.vpList.View(), "", m.width, contentHeight, true)

	notifyBar := m.notify.render(m.width)
	return titleBar + "\n" + col + "\n" + notifyBar
}

func (m Model) viewDetail() string {
	title := titleStyle.Render(m.detail.title)
	help := dimStyle.Render("j/k:scroll  g/G:top/bottom  esc/q:back")
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

	notifyBar := m.notify.render(m.width)
	return titleBar + "\n" + col + "\n" + notifyBar
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
