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

const refreshInterval = 5 * time.Second

type Model struct {
	projectRoot string
	width       int
	height      int

	activeCol int
	cursor    [3]int // per-column cursor position

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
			m.activeCol = (m.activeCol + 1) % 3
			m.updateDetail()
		case key.Matches(msg, keys.ShiftTab):
			m.activeCol = (m.activeCol + 2) % 3
			m.updateDetail()
		case key.Matches(msg, keys.Up):
			if m.cursor[m.activeCol] > 0 {
				m.cursor[m.activeCol]--
				m.updateDetail()
			}
		case key.Matches(msg, keys.Down):
			max := m.maxCursor()
			if m.cursor[m.activeCol] < max-1 {
				m.cursor[m.activeCol]++
				m.updateDetail()
			}
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

func (m *Model) maxCursor() int {
	switch m.activeCol {
	case colBacklog:
		return backlogItemCount(m.tasks, m.prs)
	case colWorkers:
		return len(m.workers)
	default:
		return 0
	}
}

func (m *Model) clampCursors() {
	if n := backlogItemCount(m.tasks, m.prs); m.cursor[colBacklog] >= n && n > 0 {
		m.cursor[colBacklog] = n - 1
	}
	if n := len(m.workers); m.cursor[colWorkers] >= n && n > 0 {
		m.cursor[colWorkers] = n - 1
	}
}

func (m *Model) updateDetail() {
	switch m.activeCol {
	case colBacklog:
		idx := m.cursor[colBacklog]
		if idx < len(m.tasks) {
			m.detail = taskDetail(m.tasks[idx], m.projectRoot)
		} else if prIdx := idx - len(m.tasks); prIdx < len(m.prs) {
			m.detail = prDetail(m.prs[prIdx])
		} else {
			m.detail = detailState{}
		}
	case colWorkers:
		idx := m.cursor[colWorkers]
		if idx < len(m.workers) {
			m.detail = workerDetail(m.workers[idx])
		} else {
			m.detail = detailState{}
		}
	}
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Title bar
	title := titleStyle.Render("Sindri — AI Agent Orchestrator")
	help := dimStyle.Render("tab:column  j/k:navigate  r:refresh  q:quit")
	titleBar := lipgloss.JoinHorizontal(lipgloss.Top,
		title,
		lipgloss.NewStyle().Width(m.width-lipgloss.Width(title)-lipgloss.Width(help)).Render(""),
		help,
	)

	contentHeight := m.height - 3

	// Three columns: 30% / 35% / 35%
	leftW := m.width * 30 / 100
	midW := m.width * 35 / 100
	rightW := m.width - leftW - midW

	left := renderBacklog(m.tasks, m.prs, m.cursor[colBacklog], leftW, contentHeight, m.activeCol == colBacklog)
	mid := renderWorkers(m.workers, m.cursor[colWorkers], midW, contentHeight, m.activeCol == colWorkers)
	right := renderDetail(m.detail, rightW, contentHeight)

	columns := lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right)

	return titleBar + "\n" + columns
}
