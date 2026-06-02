// package: tui / tui
// type:    ui
// job:     the Bubble Tea root model — backlog/workers views, navigation,
//          detail drill-down, and the refresh cycle over board.List state.
// limits:  thin renderer — data via board/issue, styling via render, mutations
//          via the action commands in actions.go (-> adapter/td, ghlocal/store).
package tui

import (
	"fmt"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/board"
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

	workers       []worker.Worker
	issues        []issue.Issue // full board (all states)
	visibleIssues []issue.Issue // issues after the active filter; rows index into this
	filter        issue.Filter
	detail        detailState

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

	pickingStatus bool
	statusOptions []string
	statusCursor  int

	// moving / movingTaskID drive the "re-parent a task in the tree" flow
	// described by view-work-list/move. When moving is true, h/l/esc are
	// routed to updateMoveMode and the source row is rendered red.
	moving        bool
	movingTaskID  string

	// loaded is false until the first tasks message has applied. Views consult
	// it to render a "Loading…" placeholder instead of the empty-state line
	// during the startup window before any data has landed.
	loaded bool

	// boardData holds the latest snapshot from each per-source loader. Each
	// refresh message updates one field and triggers a reassemble — the TUI
	// paints richer info as each loader returns instead of waiting on the
	// slowest source.
	boardData struct {
		tasks      []issue.Task
		specs      []issue.Spec
		workerByID map[string]string
		prsByID    map[string][]issue.PR
	}

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
	// Fan out all four per-source loaders in parallel; each one emits its own
	// message and the list paints as soon as the tasks loader returns (~0.3s).
	// Workers (~1.5s) and specs (~1.2s) land later without holding up the
	// first paint. warmCacheCmd fills the parent-id cache in the background.
	return tea.Batch(refreshAllCmd(m.projectRoot, false), tickCmd(), warmCacheCmd(m.projectRoot))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		return m, tea.Batch(refreshAllCmd(m.projectRoot, false), tickCmd())
	case mergeCompleteMsg, specCheckMsg, specArchivedMsg, specAbandonedMsg:
		if next, cmd, ok := m.handleSpecLifecycle(msg); ok {
			return next, cmd
		}
		return m, nil
	case cacheWarmedMsg:
		// One more tasks-only refresh so the freshly populated parent-id cache
		// gets applied; podman/openspec didn't change so don't re-poll them.
		return m, refreshTasksCmd(m.projectRoot, false)
	case movedMsg:
		// Optimistic: patch the moving task's ParentID locally, re-arrange,
		// redraw immediately. Then a tasks-only refresh confirms — no need to
		// ask podman/openspec since neither changed.
		for i := range m.issues {
			if m.issues[i].Task != nil && m.issues[i].Task.ID == msg.taskID {
				m.issues[i].Task.ParentID = msg.newParentID
				break
			}
		}
		m.issues = issue.ArrangeHierarchy(m.issues)
		m.rebuildBacklog()
		m.notify = notification{message: "Moved " + msg.taskID, time: time.Now()}
		return m, tea.Batch(flashTimer(), refreshTasksCmd(m.projectRoot, false))
	case statusChangedMsg:
		// Optimistic status update + tasks-only refresh.
		for i := range m.issues {
			if m.issues[i].Task != nil && m.issues[i].Task.ID == msg.taskID {
				m.issues[i].Task.Status = msg.newStatus
				break
			}
		}
		m.rebuildBacklog()
		m.notify = notification{message: fmt.Sprintf("Status: %s → %s", msg.prev, msg.newStatus), time: time.Now()}
		cmds := []tea.Cmd{flashTimer(), refreshTasksCmd(m.projectRoot, false)}
		// A close keeps the td task and its linked spec in sync — see
		// action.MaybeArchiveLinkedSpec for the decision logic.
		if msg.newStatus == "closed" {
			cmds = append(cmds, checkLinkedSpecCmd(m.projectRoot, msg.taskID))
		}
		return m, tea.Batch(cmds...)
	case tasksRefreshedMsg:
		m.boardData.tasks = msg.tasks
		m.reassembleIssues()
		m.loaded = true
		if msg.manual {
			m.notify = notification{
				message: fmt.Sprintf("Refreshed — %d tasks", len(m.boardData.tasks)),
				time:    time.Now(),
			}
			return m, flashTimer()
		}
		return m, nil
	case specsRefreshedMsg:
		m.boardData.specs = msg.specs
		m.reassembleIssues()
		return m, nil
	case workersRefreshedMsg:
		m.workers = msg.workers
		m.boardData.workerByID = board.WorkerByID(msg.workers)
		m.reassembleIssues()
		m.clampCursors()
		m.syncListScroll()
		return m, nil
	case prsRefreshedMsg:
		m.boardData.prsByID = msg.prs
		m.reassembleIssues()
		return m, nil
	}

	// The status picker is a global modal that overlays whatever view is
	// active, so its input dispatch must run before the per-view handlers
	// below — otherwise s pressed in the list view would be silently ignored.
	if m.pickingStatus {
		return m.updateStatusPick(msg)
	}

	if m.showCreateModal {
		return m.updateModal(msg)
	}

	// Confirm modal (merge / abandon-spec / archive-spec) is global too —
	// any active confirmation captures y/n before per-view handlers run.
	if m.confirmAction != "" {
		return m.updateConfirm(msg)
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
			m.createModal = newCreateTaskModel(m.projectRoot, m.cursorSpecName())
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
			if m.moving {
				return m.applyMove(true) // child
			}
			if m.leftView == viewBacklog {
				m.navigateInto()
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("h"))):
			if m.moving {
				return m.applyMove(false) // sibling
			}
			if m.leftView == viewBacklog {
				m.navigateOut()
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			if m.moving {
				m.cancelMove()
				return m, nil
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("m"))):
			if m.leftView == viewBacklog {
				return m.enterMoveMode()
			}
		case key.Matches(msg, keys.Enter):
			m.openDetail()
		case key.Matches(msg, keys.Yank):
			if id := m.selectedID(); id != "" {
				_ = clipboard.WriteAll(id)
				m.notify = notification{message: "Copied: " + id, time: time.Now()}
				return m, flashTimer()
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("f"))):
			if m.leftView == viewBacklog {
				m.filter = (m.filter + 1) % 3
				m.rebuildBacklog()
				m.notify = notification{message: "Filter: " + filterName(m.filter), time: time.Now()}
				return m, flashTimer()
			}
		case key.Matches(msg, keys.Status):
			taskID, status := m.statusAtCursor()
			if taskID == "" {
				m.notify = notification{message: "Status: pick a task row first (cursor isn't on a task)", isError: true, time: time.Now()}
				return m, flashTimer()
			}
			m.openStatusPicker(taskID, status)
			return m, nil
		case key.Matches(msg, keys.Approve):
			taskID, prID := m.cursorTaskAndPR()
			if taskID == "" {
				m.notify = notification{message: "Approve: pick a task row first", isError: true, time: time.Now()}
				return m, flashTimer()
			}
			if prID == "" {
				m.notify = notification{message: "Approve: this task has no PR yet", isError: true, time: time.Now()}
				return m, flashTimer()
			}
			m.detail.taskID = taskID
			m.detail.prIDs = []string{prID}
			return m, m.approvePR()
		case key.Matches(msg, keys.Reject):
			// Same key, different verb depending on row kind: on a spec-only
			// row 'x' abandons the spec (after a confirm dialog); on a task
			// row it's the existing reject flow. Spec-linked task rows fall
			// through to reject — the cursor must be on the spec row itself
			// to abandon, which keeps the destructive action explicit.
			if specName := m.cursorSpecName(); specName != "" {
				m.confirmAction = "abandon-spec:" + specName
				m.confirmLabel = fmt.Sprintf(
					"Abandon spec %s? Deletes the change folder and closes its linked open tasks. (y/n)",
					specName)
				return m, nil
			}
			taskID, prID := m.cursorTaskAndPR()
			if taskID == "" {
				m.notify = notification{message: "Reject: pick a task row first", isError: true, time: time.Now()}
				return m, flashTimer()
			}
			// Stash IDs so the shared reject flow finds them. prID may be ""
			// — rejectTask() then falls back to RejectTask (task-level reject).
			m.detail.taskID = taskID
			if prID != "" {
				m.detail.prIDs = []string{prID}
			} else {
				m.detail.prIDs = nil
			}
			ti := textinput.New()
			ti.Placeholder = "Reason for rejection..."
			ti.Focus()
			ti.CharLimit = 500
			ti.Width = m.width - 20
			m.rejecting = true
			m.commentInput = ti
			return m, textinput.Blink
		case key.Matches(msg, keys.Refresh):
			return m, refreshAllCmd(m.projectRoot, true)
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

func filterName(f issue.Filter) string {
	switch f {
	case issue.FilterAll:
		return "all"
	case issue.FilterClosed:
		return "closed only"
	default:
		return "open (closed hidden)"
	}
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
			if row.issueIdx < len(m.visibleIssues) {
				return m.visibleIssues[row.issueIdx].ID()
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
			} else if row.issueIdx < len(m.visibleIssues) {
				m.detail = issueDetail(m.visibleIssues[row.issueIdx], m.projectRoot)
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

// reassembleIssues runs issue.Assemble over the current boardData snapshot,
// updates m.issues, rebuilds the backlog, and re-renders the detail pane if
// the currently-shown task is still present. Called by every per-source
// refresh handler so the UI re-computes the moment any source updates.
func (m *Model) reassembleIssues() {
	m.issues = issue.Assemble(m.boardData.tasks, m.boardData.specs, m.boardData.workerByID, m.boardData.prsByID)
	m.rebuildBacklog()
	if m.showDetail && m.detail.taskID != "" {
		for _, iss := range m.issues {
			if iss.ID() == m.detail.taskID {
				m.detail = issueDetail(iss, m.projectRoot)
				m.vpDetail.SetContent(m.detail.content)
				break
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
		return m, tea.Batch(refreshAllCmd(m.projectRoot, false), flashTimer())
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
	} else if m.pickingStatus {
		bottomBar = lipgloss.NewStyle().PaddingLeft(1).Render(renderStatusPicker(m.statusOptions, m.statusCursor))
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
