package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
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
	backlogPanel int // panelTasks or panelPRs
	taskCursor   int
	prCursor     int
	workerCursor int

	workers []worker.Worker
	tasks   []taskItem
	prs     []prItem
	detail  detailState
}

func New(projectRoot string) Model {
	return Model{
		projectRoot: projectRoot,
		activeCol:   colBacklog,
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
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Tab):
			if m.activeCol == colBacklog {
				m.activeCol = colWorkers
			} else {
				m.activeCol = colBacklog
			}
			m.updateDetail()
		case key.Matches(msg, keys.ShiftTab):
			if m.activeCol == colWorkers {
				m.activeCol = colBacklog
			} else {
				m.activeCol = colWorkers
			}
			m.updateDetail()
		case key.Matches(msg, keys.PanelSwitch):
			if m.activeCol == colBacklog {
				if m.backlogPanel == panelTasks {
					m.backlogPanel = panelPRs
				} else {
					m.backlogPanel = panelTasks
				}
				m.updateDetail()
			}
		case key.Matches(msg, keys.DetailUp):
			m.detail.scrollUp()
		case key.Matches(msg, keys.DetailDown):
			contentHeight := m.height - 3
			m.detail.scrollDown(contentHeight)
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
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	title := titleStyle.Render("Sindri — AI Agent Orchestrator")
	help := dimStyle.Render("tab:column  g:panel  j/k:list  J/K:detail  r:refresh  q:quit")
	titleBar := lipgloss.JoinHorizontal(lipgloss.Top,
		title,
		lipgloss.NewStyle().Width(m.width-lipgloss.Width(title)-lipgloss.Width(help)).Render(""),
		help,
	)

	contentHeight := m.height - 3

	leftW := m.width * 30 / 100
	midW := m.width * 35 / 100
	rightW := m.width - leftW - midW

	isActive := m.activeCol == colBacklog
	left := renderBacklogSplit(m.tasks, m.prs, m.taskCursor, m.prCursor, m.backlogPanel, leftW, contentHeight, isActive)
	mid := renderWorkers(m.workers, m.workerCursor, midW, contentHeight, m.activeCol == colWorkers)
	right := renderDetail(m.detail, rightW, contentHeight)

	columns := lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right)

	return titleBar + "\n" + columns
}
