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
	colLeft = iota
	colDetail
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

	focusCol int // colLeft or colDetail
	leftView int // viewBacklog or viewWorkers

	listCursor   int
	backlogRows  []backlogRow
	workerCursor int

	workers []worker.Worker
	tasks   []taskItem
	prs     []prItem
	detail  detailState

	vpLeft   viewport.Model
	vpDetail viewport.Model

	showCreateModal bool
	createModal     createTaskModel
}

func New(projectRoot string) Model {
	return Model{
		projectRoot: projectRoot,
		focusCol:    colLeft,
		leftView:    viewBacklog,
		vpLeft:      viewport.New(0, 0),
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
		case key.Matches(msg, keys.NewTask):
			m.showCreateModal = true
			m.createModal = newCreateTaskModel(m.projectRoot)
			return m, m.createModal.Init()
		case key.Matches(msg, keys.Backlog):
			m.leftView = viewBacklog
			m.focusCol = colLeft
			m.rebuildBacklog()
			m.updateDetail()
		case key.Matches(msg, keys.Workers):
			m.leftView = viewWorkers
			m.focusCol = colLeft
			m.updateDetail()
		case key.Matches(msg, keys.NavRight):
			m.focusCol = colDetail
		case key.Matches(msg, keys.NavLeft):
			m.focusCol = colLeft
		case key.Matches(msg, keys.DetailUp):
			m.vpDetail.LineUp(1)
		case key.Matches(msg, keys.DetailDown):
			m.vpDetail.LineDown(1)
		case key.Matches(msg, keys.Up):
			if m.focusCol == colLeft {
				m.moveCursor(-1)
			}
		case key.Matches(msg, keys.Down):
			if m.focusCol == colLeft {
				m.moveCursor(1)
			}
		case key.Matches(msg, keys.Refresh):
			return m, refreshData(m.projectRoot)
		}

	case refreshMsg:
		m.workers = msg.workers
		m.tasks = msg.tasks
		m.prs = msg.prs
		m.rebuildBacklog()
		m.clampCursors()
		m.updateDetail()

	case tickMsg:
		return m, tea.Batch(refreshData(m.projectRoot), tickCmd())
	}

	return m, nil
}

func (m *Model) resizeViewports() {
	contentHeight := m.height - 3
	innerH := contentHeight - 4 // border(2) + header(2)
	if innerH < 1 {
		innerH = 1
	}
	leftW := m.width * 2 / 3
	rightW := m.width - leftW
	m.vpLeft.Width = leftW - 4
	m.vpLeft.Height = innerH
	m.vpDetail.Width = rightW - 4
	m.vpDetail.Height = innerH - 1 // room for scroll status
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
		next := m.listCursor + delta
		if next >= 0 && next < len(m.backlogRows) {
			m.listCursor = next
			m.updateDetail()
		}
	case viewWorkers:
		next := m.workerCursor + delta
		if next >= 0 && next < len(m.workers) {
			m.workerCursor = next
			m.updateDetail()
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

func (m *Model) updateDetail() {
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
					m.detail = taskDetail(m.tasks[row.taskIdx], m.projectRoot)
				}
			}
		} else {
			m.detail = detailState{}
		}
	case viewWorkers:
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

	// Title bar
	title := titleStyle.Render("Sindri — AI Agent Orchestrator")
	viewLabel := "backlog"
	if m.leftView == viewWorkers {
		viewLabel = "workers"
	}
	help := dimStyle.Render(fmt.Sprintf("[%s]  b/w:view  j/k:list  J/K:detail  C-h/l:focus  n:new  r:refresh  q:quit", viewLabel))
	titleBar := lipgloss.JoinHorizontal(lipgloss.Top,
		title,
		lipgloss.NewStyle().Width(m.width-lipgloss.Width(title)-lipgloss.Width(help)).Render(""),
		help,
	)

	contentHeight := m.height - 3
	leftW := m.width * 2 / 3
	rightW := m.width - leftW

	// Left column content
	var leftContent string
	var leftHeader string
	switch m.leftView {
	case viewBacklog:
		leftHeader = "Backlog"
		leftContent = renderBacklogList(m.backlogRows, m.listCursor, m.focusCol == colLeft)
	case viewWorkers:
		leftHeader = "Workers"
		leftContent = renderWorkersList(m.workers, m.workerCursor, m.focusCol == colLeft)
	}
	m.vpLeft.SetContent(leftContent)

	// Detail scroll status
	scrollStatus := ""
	if m.vpDetail.TotalLineCount() > m.vpDetail.Height {
		pct := int(m.vpDetail.ScrollPercent() * 100)
		scrollStatus = dimStyle.Render(fmt.Sprintf(" %d%% (%d/%d)", pct, m.vpDetail.YOffset+m.vpDetail.Height, m.vpDetail.TotalLineCount()))
	}

	left := renderColumn(leftHeader, m.vpLeft.View(), "", leftW, contentHeight, m.focusCol == colLeft)
	right := renderColumn("Detail", m.vpDetail.View(), scrollStatus, rightW, contentHeight, m.focusCol == colDetail)

	columns := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
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
