package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/worker"
)

const (
	colBacklog = iota
	colWorkers
	colDetail
)

const (
	panelTasks = iota
	panelPRs
)

const refreshInterval = 5 * time.Second

type Model struct {
	projectRoot string
	width       int
	height      int

	activeCol    int
	backlogPanel int
	taskCursor   int
	prCursor     int
	workerCursor int

	workers []worker.Worker
	tasks   []taskItem
	prs     []prItem
	detail  detailState

	vpBacklog viewport.Model
	vpWorkers viewport.Model
	vpDetail  viewport.Model

	showCreateModal bool
	createModal     createTaskModel
}

func New(projectRoot string) Model {
	return Model{
		projectRoot: projectRoot,
		activeCol:   colBacklog,
		vpBacklog:   viewport.New(0, 0),
		vpWorkers:   viewport.New(0, 0),
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

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewports()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, key.NewBinding(key.WithKeys("n"))):
			m.showCreateModal = true
			m.createModal = newCreateTaskModel(m.projectRoot)
			return m, m.createModal.Init()
		case key.Matches(msg, keys.NavRight):
			if m.activeCol == colBacklog {
				m.activeCol = colWorkers
				m.updateDetail()
			}
		case key.Matches(msg, keys.NavLeft):
			if m.activeCol == colWorkers {
				m.activeCol = colBacklog
				m.updateDetail()
			}
		case key.Matches(msg, keys.NavDown):
			if m.activeCol == colBacklog && m.backlogPanel == panelTasks {
				m.backlogPanel = panelPRs
				m.updateDetail()
			}
		case key.Matches(msg, keys.NavUp):
			if m.activeCol == colBacklog && m.backlogPanel == panelPRs {
				m.backlogPanel = panelTasks
				m.updateDetail()
			}
		case key.Matches(msg, keys.DetailUp):
			m.vpDetail.LineUp(1)
		case key.Matches(msg, keys.DetailDown):
			m.vpDetail.LineDown(1)
		case key.Matches(msg, keys.Up):
			m.moveCursor(-1)
		case key.Matches(msg, keys.Down):
			m.moveCursor(1)
		case key.Matches(msg, keys.Refresh):
			return m, refreshData(m.projectRoot)
		}

	case refreshMsg:
		m.workers = msg.workers
		m.tasks = msg.tasks
		m.prs = msg.prs
		m.clampCursors()
		m.updateDetail()

	case tickMsg:
		return m, tea.Batch(refreshData(m.projectRoot), tickCmd())
	}

	return m, nil
}

func (m *Model) resizeViewports() {
	contentHeight := m.height - 3
	// Border (2) + header with padding (2) = 4 lines overhead per column
	innerH := contentHeight - 4
	if innerH < 1 {
		innerH = 1
	}
	leftW := m.width * 30 / 100
	midW := m.width * 35 / 100
	rightW := m.width - leftW - midW
	// Subtract border (2) + horizontal padding (2) for inner width
	m.vpBacklog.Width = leftW - 4
	m.vpBacklog.Height = innerH
	m.vpWorkers.Width = midW - 4
	m.vpWorkers.Height = innerH
	m.vpDetail.Width = rightW - 4
	// Detail gets 1 less for scroll status line
	m.vpDetail.Height = innerH - 1
	if m.vpDetail.Height < 1 {
		m.vpDetail.Height = 1
	}
}

func (m *Model) moveCursor(delta int) {
	switch m.activeCol {
	case colBacklog:
		if m.backlogPanel == panelTasks {
			next := m.taskCursor + delta
			if next >= 0 && next < len(m.tasks) {
				m.taskCursor = next
				m.updateDetail()
			}
		} else {
			next := m.prCursor + delta
			if next >= 0 && next < len(m.prs) {
				m.prCursor = next
				m.updateDetail()
			}
		}
	case colWorkers:
		next := m.workerCursor + delta
		if next >= 0 && next < len(m.workers) {
			m.workerCursor = next
			m.updateDetail()
		}
	}
}

func (m *Model) clampCursors() {
	if n := len(m.tasks); m.taskCursor >= n && n > 0 {
		m.taskCursor = n - 1
	}
	if n := len(m.prs); m.prCursor >= n && n > 0 {
		m.prCursor = n - 1
	}
	if n := len(m.workers); m.workerCursor >= n && n > 0 {
		m.workerCursor = n - 1
	}
}

func (m *Model) updateDetail() {
	switch m.activeCol {
	case colBacklog:
		if m.backlogPanel == panelTasks {
			if m.taskCursor < len(m.tasks) {
				m.detail = taskDetail(m.tasks[m.taskCursor], m.projectRoot)
			} else {
				m.detail = detailState{}
			}
		} else {
			if m.prCursor < len(m.prs) {
				m.detail = prDetail(m.prs[m.prCursor])
			} else {
				m.detail = detailState{}
			}
		}
	case colWorkers:
		if m.workerCursor < len(m.workers) {
			m.detail = workerDetail(m.workers[m.workerCursor])
		} else {
			m.detail = detailState{}
		}
	}
	m.vpDetail.SetContent(m.detail.content)
	m.vpDetail.GotoTop()
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
		return m, refreshData(m.projectRoot)
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

	title := titleStyle.Render("Sindri — AI Agent Orchestrator")
	help := dimStyle.Render("C-hjkl:navigate  j/k:list  J/K:detail  n:new  r:refresh  q:quit")
	titleBar := lipgloss.JoinHorizontal(lipgloss.Top,
		title,
		lipgloss.NewStyle().Width(m.width-lipgloss.Width(title)-lipgloss.Width(help)).Render(""),
		help,
	)

	contentHeight := m.height - 3
	leftW := m.width * 30 / 100
	midW := m.width * 35 / 100
	rightW := m.width - leftW - midW

	// Build content for viewports
	m.vpBacklog.SetContent(renderBacklogContent(m.tasks, m.prs, m.taskCursor, m.prCursor, m.backlogPanel, m.activeCol == colBacklog))
	m.vpWorkers.SetContent(renderWorkersContent(m.workers, m.workerCursor, m.activeCol == colWorkers))

	// Build detail scroll status
	detailContent := m.vpDetail.View()
	scrollStatus := ""
	if m.vpDetail.TotalLineCount() > m.vpDetail.Height {
		pct := int(m.vpDetail.ScrollPercent() * 100)
		scrollStatus = dimStyle.Render(fmt.Sprintf(" %d%% (%d/%d) J/K:scroll", pct, m.vpDetail.YOffset+m.vpDetail.Height, m.vpDetail.TotalLineCount()))
	}

	// Render columns: border wraps viewport
	left := renderColumn("Backlog", m.vpBacklog.View(), "", leftW, contentHeight, m.activeCol == colBacklog)
	mid := renderColumn("Workers", m.vpWorkers.View(), "", midW, contentHeight, m.activeCol == colWorkers)
	right := renderColumn("Detail", detailContent, scrollStatus, rightW, contentHeight, false)

	columns := lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right)
	return titleBar + "\n" + columns
}

func renderColumn(header, content, footer string, width, height int, active bool) string {
	style := columnStyle.Width(width).Height(height)
	if active {
		style = activeColumnStyle.Width(width).Height(height)
	}
	full := headerStyle.Render(header) + "\n" + content
	if footer != "" {
		full += "\n" + footer
	}
	return style.Render(full)
}

// renderBacklogContent builds the plain text content for the backlog viewport.
func renderBacklogContent(tasks []taskItem, prs []prItem, taskCursor, prCursor, activePanel int, active bool) string {
	var b strings.Builder

	taskHeader := "Tasks"
	if active && activePanel == panelTasks {
		taskHeader = "Tasks ●"
	}
	b.WriteString(headerStyle.Render(taskHeader))
	b.WriteByte('\n')

	if len(tasks) == 0 {
		b.WriteString(dimStyle.Render("No tasks"))
		b.WriteByte('\n')
	} else {
		for i, t := range tasks {
			line := formatTaskLine(t)
			if active && activePanel == panelTasks && i == taskCursor {
				b.WriteString(selectedItemStyle.Render("> " + line))
			} else {
				b.WriteString("  " + line)
			}
			b.WriteByte('\n')
		}
	}

	b.WriteByte('\n')

	prHeader := "Pull Requests"
	if active && activePanel == panelPRs {
		prHeader = "Pull Requests ●"
	}
	b.WriteString(headerStyle.Render(prHeader))
	b.WriteByte('\n')

	if len(prs) == 0 {
		b.WriteString(dimStyle.Render("No PRs"))
		b.WriteByte('\n')
	} else {
		for i, pr := range prs {
			line := formatPRLine(pr)
			if active && activePanel == panelPRs && i == prCursor {
				b.WriteString(selectedItemStyle.Render("> " + line))
			} else {
				b.WriteString("  " + line)
			}
			b.WriteByte('\n')
		}
	}

	return b.String()
}

// renderWorkersContent builds the plain text content for the workers viewport.
func renderWorkersContent(workers []worker.Worker, selected int, active bool) string {
	var b strings.Builder
	if len(workers) == 0 {
		b.WriteString(dimStyle.Render("No workers"))
		b.WriteByte('\n')
	}
	for i, wk := range workers {
		line := formatWorkerLine(wk)
		if active && i == selected {
			b.WriteString(selectedItemStyle.Render("> " + line))
		} else {
			b.WriteString("  " + line)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
