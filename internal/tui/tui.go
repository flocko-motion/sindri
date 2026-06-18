// package: tui / tui
// type:    ui (a thin hub client)
// job:     the dashboard shell — model, update loop, and the full-height
//          master-detail View that composes the generic components
//          (component_*.go) around the per-tab content (tasks.go/agents.go/
//          prs.go). Live over /events; all derivation comes from the hub.
// limits:  no domain logic; hub client only; refuses to start without a hub.
package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/flo-at/sindri/internal/tui/scroll"
)

// Run starts the dashboard against the repo's hub (refuses without one).
func Run(root string) error {
	// Startup breadcrumbs to stderr (before the alt screen takes over) so a hang
	// is attributable to a step rather than silent.
	fmt.Fprintf(os.Stderr, "sindri tui: hub at %s\n", hub.SocketPath(root))
	if !hub.IsRunning(root) {
		return fmt.Errorf("no hub running — start one first: 'sindri hub &'")
	}
	fmt.Fprintln(os.Stderr, "sindri tui: connecting to /events…")
	cl := client.Dial(root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := cl.Watch(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "sindri tui: connected — starting dashboard")
	_, err = tea.NewProgram(newModel(cl, ch, root), tea.WithAltScreen()).Run()
	return err
}

const (
	filterOpen = iota
	filterClosed
	filterAll
)

var filterNames = [...]string{"open", "closed", "all"}

// inputMode is the active text-input modal (none = normal navigation).
type inputMode int

const (
	inputNone inputMode = iota
	inputTell
)

type model struct {
	cl    *client.HTTP
	ch    <-chan hub.BoardState
	root  string // repo root — needed to build repo-scoped podman container names
	state hub.BoardState
	err   error
	w, h  int

	tab    int
	cursor [3]int
	list   scroll.Viewport
	detail scroll.Viewport

	filter     int
	collapsed  map[string]bool
	hideDetail bool // § force-hides the detail pane (else shown when wide enough)

	rightFocus  bool // detail (right) column has focus (h/l switch; j/k move within)
	rightCursor int  // focused actionable item in the right column

	detailKey  string
	agentLog   []store.Event
	agentPane  string // captured tmux screen of the selected agent (live)
	agentView  string // Agents main pane: "screen" (tmux, default) | "pod" (podman info)
	agentPod   string // fetched podman pod-info for the selected agent
	prDetail     hub.PRDetail
	prView       string // which content the PR big pane shows: "diff" (default) | "lint"
	reviewPrompt string // editable default review instruction (from the hub)
	taskDetail   store.Task
	quit       bool

	modalOverride      []string // when set, the detail modal shows these instead of the tab detail
	modalOverrideTitle string

	mode        inputMode // active text-input modal
	input       textinput.Model
	inputTarget string    // selection captured when the modal opened
	modal       bool      // detail modal (full-screen) is open
	choice      choiceModalState
	form        formState // active fill-in form (new/edit task)
	flash       string    // transient status (e.g. "copied"), cleared on next key
	errText     string    // when set, the error modal is shown (any key dismisses)
}

// choiceModalState is a generic pick-one prompt: options, parallel values, and
// what to do with the chosen value.
type choiceModalState struct {
	active  bool
	title   string
	options []string
	values  []string
	cursor  int
	apply   func(value string) tea.Cmd
}

// detailMinWidth is the narrowest terminal that still shows the inline detail
// pane; below it the selector goes full-width and detail is ENTER-only.
const detailMinWidth = 135

// wide reports whether the terminal is wide enough for a side detail column.
func (m model) wide() bool { return m.w >= detailMinWidth }

// showDetail reports whether the right detail column should be shown (wide
// enough and not §-hidden). The Agents/PRs left split still renders when wide
// even if the detail is hidden — § only drops the right column there.
func (m model) showDetail() bool { return m.wide() && !m.hideDetail }

func newModel(cl *client.HTTP, ch <-chan hub.BoardState, root string) model {
	// Default to a sane size so a frame renders immediately — the real size
	// arrives via WindowSizeMsg and resizes. (Some terminals send the initial
	// size late or as 0×0; without a default the view would stick on "loading".)
	in := textinput.New()
	in.CharLimit = 200
	m := model{cl: cl, ch: ch, root: root, collapsed: map[string]bool{}, w: 80, h: 24, input: in}
	m.reclamp()
	return m
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{waitForState(m.ch), tickCmd()}
	if m.cl != nil { // load the editable default review prompt
		cl := m.cl
		cmds = append(cmds, func() tea.Msg {
			p, err := cl.ReviewPrompt()
			if err != nil {
				return nil
			}
			return reviewPromptMsg(p)
		})
	}
	return tea.Batch(cmds...)
}

func waitForState(ch <-chan hub.BoardState) tea.Cmd {
	return func() tea.Msg {
		st, ok := <-ch
		if !ok {
			return errMsg{fmt.Errorf("hub connection closed")}
		}
		return stateMsg(st)
	}
}

func (m model) bodyHeight() int {
	if h := m.h - 3; h > 0 { // tab strip (1) + footer (2)
		return h
	}
	return 1
}

func (m model) leftWidth() int {
	// The Tasks table (gutter + id + type + prio + state + title) needs room, so
	// give the selector ~60% — clamped so neither pane gets too narrow.
	w := m.w * 3 / 5
	return clampInt(w, 28, max(28, m.w-28))
}

func (m model) detailWidth() int {
	w := m.w - m.leftWidth() - 1 // 1 for the divider
	if w < 1 {
		w = 1
	}
	return w
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 && msg.Height > 0 { // ignore bogus 0×0 (keeps the default)
			m.w, m.h = msg.Width, msg.Height
		}
		m.reclamp()
		if m.modal {
			m.detail.SetHeight(modalContentHeight(m.h))
		}
	case stateMsg:
		m.state = hub.BoardState(msg)
		m.reclamp()
		return m, tea.Batch(waitForState(m.ch), m.syncDetail(), m.agentLiveCmds())
	case polledMsg: // an auto-refresh poll — update the board, don't touch the SSE waiter
		m.state = hub.BoardState(msg)
		m.reclamp()
		return m, tea.Batch(m.syncDetail(), m.agentLiveCmds())
	case tickMsg:
		// Live agent state (status), screen, and log go stale between hub
		// notifications; while on the Agents tab, poll every few seconds. The
		// state poll cascades to log+screen refetches via polledMsg.
		cmds := []tea.Cmd{tickCmd()}
		if m.tab == 1 && m.cl != nil {
			cmds = append(cmds, pollStateCmd(m.cl))
		}
		return m, tea.Batch(cmds...)
	case logMsg:
		if msg.key == m.selID() { // ignore a stale fetch from a prior selection
			m.agentLog = msg.evs
		}
	case paneMsg:
		if msg.agent == m.selID() { // ignore a stale capture from a prior selection
			m.agentPane = msg.text
		}
	case agentPodMsg:
		if msg.agent == m.selID() {
			m.agentPod = msg.text
		}
	case prLintMsg:
		if msg.pr == m.selID() { // store the result, switch to the lint view, focus it
			m.prDetail.Lint = msg.text
			m.prView = "lint"
			m.rightFocus = true
			m.rightCursor = m.viewCursor("lint")
			m.detail.Resize(m.detail.Height, len(m.prContentLines()))
		}
	case reviewPromptMsg:
		m.reviewPrompt = string(msg)
	case reviewReadyMsg: // PR materialized — drop into a shell in the review workspace
		return m, tea.ExecProcess(shellAt(string(msg)), func(error) tea.Msg { return nil })
	case prMsg:
		m.prDetail = msg.d
	case taskMsg:
		m.taskDetail = msg.t
	case errModalMsg:
		m.errText = msg.err.Error()
	case errMsg:
		m.err = msg.err
		return m, tea.Quit
	case tea.KeyMsg:
		if m.errText != "" { // any key dismisses the error modal
			m.errText = ""
			return m, nil
		}
		if m.form.active {
			return m, m.form.update(msg)
		}
		if m.mode != inputNone {
			return m.updateInput(msg)
		}
		if m.choice.active {
			return m.updateChoice(msg)
		}
		if m.modal {
			return m.updateModal(msg)
		}
		cmd := m.onKey(msg.String())
		if m.quit {
			return m, tea.Quit
		}
		return m, cmd
	}
	return m, nil
}



// modalTitle labels the detail modal for the current selection.
func (m model) modalTitle() string {
	switch m.tab {
	case 0:
		return "Task " + m.selID()
	case 1:
		return "Agent " + m.selID()
	default:
		return "PR " + m.selID()
	}
}


// onKey applies a key (by its string form) — shared by the live loop and the
// headless Screenshot harness. Mutates the model; returns an optional cmd.
func (m *model) onKey(k string) tea.Cmd {
	oldTab := m.tab
	m.flash = "" // any keypress clears the previous transient status
	switch k {
	case "y": // yank: the focused right-column value, else the selected id
		if m.rightFocus {
			if it, ok := m.focusedItem(); ok {
				_ = clipboard.WriteAll(it.value)
				m.flash = "copied: " + it.value
			}
			return nil
		}
		if id := m.selID(); id != "" {
			_ = clipboard.WriteAll(id)
			m.flash = "copied id: " + id
		}
		return nil
	case "Y": // yank the selection's full details
		if m.selID() != "" {
			_ = clipboard.WriteAll(strings.Join(m.detailLines(), "\n"))
			m.flash = "copied full details"
		}
		return nil
	}
	switch k {
	case "q", "ctrl+c":
		m.quit = true
		return nil
	case "tab": // the only way to switch tabs (with shift+tab)
		m.tab = (m.tab + 1) % len(hub.Sections)
	case "shift+tab":
		m.tab = (m.tab - 1 + len(hub.Sections)) % len(hub.Sections)
	case "ctrl+l": // the only way to switch panes (with ctrl+h): focus the detail
		if m.showDetail() && len(m.actionableItems()) > 0 {
			m.rightFocus = true
			m.rightCursor = clampInt(m.rightCursor, 0, max(0, len(m.actionableItems())-1))
		}
	case "ctrl+h": // focus back to the list
		m.rightFocus = false
	case "j", "down":
		if m.rightFocus {
			m.rightCursor = clampInt(m.rightCursor+1, 0, max(0, len(m.actionableItems())-1))
		} else {
			m.cursor[m.tab]++
		}
	case "k", "up":
		if m.rightFocus {
			m.rightCursor = clampInt(m.rightCursor-1, 0, max(0, len(m.actionableItems())-1))
		} else {
			m.cursor[m.tab]--
		}
	case "J": // scroll the detail pane down (yazi-style secondary-pane scroll)
		for i := 0; i < detailScrollStep; i++ {
			m.detail.ScrollDown()
		}
		return nil
	case "K": // scroll the detail pane up
		for i := 0; i < detailScrollStep; i++ {
			m.detail.ScrollUp()
		}
		return nil
	case "g": // goto the focused cross-reference's home, else jump the list to top
		if m.rightFocus {
			// gotoItem may switch tabs; fall through to the tail reclamp + syncDetail
			// so the destination tab's viewports are sized for it (not the old tab).
			if it, ok := m.focusedItem(); ok && it.kind != "path" {
				m.gotoItem(it.kind, it.value)
			}
		} else {
			m.cursor[m.tab] = 0
		}
	case "G":
		m.cursor[m.tab] = 1 << 30
	case "ctrl+d":
		m.cursor[m.tab] += m.bodyHeight() / 2
	case "ctrl+u":
		m.cursor[m.tab] -= m.bodyHeight() / 2
	case "f":
		if m.tab == 0 {
			m.filter = (m.filter + 1) % 3
		}
	case "h": // tasks: collapse the fold under the cursor (tree navigation)
		if m.tab == 0 && !m.rightFocus {
			if id := m.selID(); id != "" {
				m.collapsed[id] = true
			}
		}
	case "l": // tasks: expand the fold under the cursor (tree navigation)
		if m.tab == 0 && !m.rightFocus {
			delete(m.collapsed, m.selID())
		}
	case "S": // agents: Start/Stop toggle — start if down, stop if running
		if m.tab == 1 {
			return m.agentStartStop()
		}
	case "a": // agents: attach to the live tmux session (out-of-band)
		if m.tab == 1 {
			if a, ok := m.selAgent(); ok {
				if a.Status == "down" {
					m.errText = "agent " + a.Name + " is down — start it first ('S') before attaching"
					return nil
				}
				if m.cl != nil {
					return tea.ExecProcess(attachCmd(m.root, a.Name), func(error) tea.Msg { return nil })
				}
			}
		}
	case "m": // prs: merge (the human gate)
		if m.tab == 2 {
			return m.action(func(id string) error { _, err := m.cl.Merge(id); return err })
		}
	case "N": // new task (tasks) / new agent (agents)
		if m.tab == 0 {
			m.openTaskForm(false)
			return nil
		} else if m.tab == 1 { // agents: pick the role, then auto-name after a dwarf
			m.openNewAgentChoice()
			return nil
		}
	case "e": // edit the selected task (agents have no editable fields — role is fixed at creation)
		if m.tab == 0 && m.selID() != "" {
			m.openTaskForm(true)
			return nil
		}
	case "D": // delete the selected agent (with confirm)
		if m.tab == 1 && m.selID() != "" {
			m.openDeleteChoice(m.selID())
			return nil
		}
	case "t": // tell the selected agent (agents) / show linked task (prs)
		if m.tab == 1 && m.selID() != "" {
			m.openInput(inputTell, "tell "+m.selID()+": ")
			return textinput.Blink
		} else if m.tab == 2 {
			if d := m.prDetail; d.PR.ID == m.selID() && d.Task.ID != "" {
				m.openTaskModal(d.Task)
			}
			return nil
		}
	case "L": // prs: run the quality gate against the PR's worktree
		if m.tab == 2 {
			if id := m.selID(); id != "" && m.cl != nil {
				return m.lintCmd(id)
			}
		}
	case "R": // prs: reject a PR · tasks: reject a planner-proposed task (with a comment)
		if m.tab == 2 && m.selID() != "" {
			m.openRejectForm(m.selID())
			return nil
		}
		if m.tab == 0 && m.taskGated() {
			m.openTaskRejectForm(m.selID())
			return nil
		}
	case "V": // prs: verify — materialize the PR into the review workspace + shell in
		if m.tab == 2 {
			if id := m.selID(); id != "" && m.cl != nil {
				return m.verifyCmd(id)
			}
		}
	case "A": // prs: request an agentic review · tasks: approve a planner-proposed task
		if m.tab == 2 && m.selID() != "" {
			m.openReviewForm(m.selID())
			return nil
		}
		if m.tab == 0 && m.taskGated() {
			return m.approveTaskCmd(m.selID())
		}
	case "p": // set the selected task's priority
		if m.tab == 0 && m.selID() != "" {
			m.openPriorityChoice(m.selID())
			return nil
		}
	case "U": // tasks: release the selected task back to the backlog
		if m.tab == 0 && m.selID() != "" {
			return m.unassignTaskCmd(m.selID())
		}
	case "enter":
		if m.rightFocus { // act on the focused detail item
			if it, ok := m.focusedItem(); ok {
				switch it.kind {
				case "view": // switch the big content pane
					if m.tab == 1 { // Agents: toggle live screen ⇄ pod info
						if m.agentView == "pod" {
							m.agentView = "screen"
							return nil
						}
						m.agentView = "pod"
						if m.cl != nil {
							return podFetchCmd(m.cl, m.selID())
						}
						return nil
					}
					m.prView = it.value // PRs: diff ⇄ lint
					m.detail.Resize(m.detail.Height, len(m.prContentLines()))
				case "path": // open a shell in the workspace
					return tea.ExecProcess(shellAt(it.value), func(error) tea.Msg { return nil })
				default: // cross-reference: open its details modal
					m.openItemModal(it.kind, it.value)
				}
			}
			return nil
		}
		if m.selID() != "" { // open the full-screen detail modal
			m.modal = true
			m.detail.SetHeight(modalContentHeight(m.h))
			m.detail.SetTotal(len(m.detailLines()))
			m.detail.ScrollTop()
			return nil
		}
	case "§": // toggle the detail pane (full-width selector when hidden)
		m.hideDetail = !m.hideDetail
		if m.hideDetail {
			m.rightFocus = false // can't focus a hidden pane
		}
	case "r":
		m.reclamp()
		return m.refreshCmd()
	}
	m.reclamp()
	cmd := m.syncDetail()
	if m.tab != oldTab { // changing tabs: drop right-column focus, auto-refresh
		m.rightFocus, m.rightCursor = false, 0
		return tea.Batch(cmd, m.refreshCmd())
	}
	return cmd
}



// reclamp keeps the active tab's cursor + both viewports in range.
func (m *model) reclamp() {
	n := len(m.rows())
	m.cursor[m.tab] = clampInt(m.cursor[m.tab], 0, max(0, n-1))
	listH := m.bodyHeight()
	if m.wide() { // agents/prs: the list is the short top region of a split
		switch m.tab {
		case 1:
			listH = m.agentListHeight()
		case 2:
			listH = m.prListHeight()
		}
	}
	m.list.SetHeight(listH)
	m.list.SetTotal(n)
	m.list.SetCursor(m.cursor[m.tab])
	// Offset-driven scroll (J/K), preserved across re-layouts; reset to top only
	// when the selection changes (syncDetail).
	if m.tab == 2 && m.wide() { // PRs: detail pane is the big bottom-left content
		m.detail.Resize(max(1, m.bodyHeight()-m.prListHeight()-1), len(m.prContentLines()))
	} else {
		m.detail.Resize(m.bodyHeight(), len(m.detailLines()))
	}
}

// syncDetail fetches the selected item's rich detail when the selection changes.
func (m *model) syncDetail() tea.Cmd {
	key := fmt.Sprintf("%d:%s", m.tab, m.selID())
	if key == m.detailKey || m.cl == nil {
		return nil
	}
	m.detailKey = key
	m.detail.ScrollTop()  // new selection → show its detail from the top
	m.rightCursor = 0     // and reset the right-column cursor to its first item
	id := m.selID()
	if id == "" {
		return nil
	}
	cl := m.cl
	switch m.tab {
	case 0:
		return func() tea.Msg { t, _ := cl.TaskInfo(id); return taskMsg{id, t} }
	case 1:
		m.agentPane, m.agentPod = "", "" // selection changed — drop the previous agent's screen/pod
		m.agentView = "screen"            // default back to the live screen
		return tea.Batch(
			func() tea.Msg { evs, _ := cl.Log(id); return logMsg{id, evs} },
			paneFetchCmd(cl, id),
		)
	default:
		m.prView = "diff" // new PR → show its diff (its stored lint loads via PRInfo)
		return func() tea.Msg { d, _ := cl.PRInfo(id); return prMsg{id, d} }
	}
}

// rows / detailLines dispatch to the active tab (tasks.go / agents.go / prs.go).
func (m model) rows() []row {
	switch m.tab {
	case 0:
		return m.taskRows()
	case 1:
		return m.agentRows()
	default:
		return m.prRows()
	}
}

func (m model) detailLines() []string {
	switch m.tab {
	case 0:
		return m.taskDetailLines()
	case 1:
		return m.agentDetailLines()
	default:
		return m.prDetailLines()
	}
}

func (m model) contextFooter() string {
	if m.rightFocus { // focused on a detail cross-reference (Tasks/PRs)
		return "j/k item · enter details · g goto · y copy"
	}
	switch m.tab {
	case 0:
		return fmt.Sprintf("N new · e edit · p priority · U unassign · A/R approve/reject · f filter: %s · h/l fold", filterNames[m.filter])
	case 1:
		return "N new · S start/stop · t tell · a attach · D delete"
	default:
		return "V verify · A agent-review · R reject · L lint · m merge"
	}
}

func (m model) selID() string {
	r := m.rows()
	if c := m.cursor[m.tab]; c >= 0 && c < len(r) {
		return r[c].id
	}
	return ""
}


// View composes the full-height frame: tab strip, master-detail body, footer.
func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("hub connection lost: %v\n", m.err)
	}
	if m.w == 0 || m.h == 0 {
		return "loading…"
	}
	labels := make([]string, len(hub.Sections))
	for i, s := range hub.Sections {
		labels[i] = fmt.Sprintf("%d %s", s.Count(m.state), s.Title)
	}
	// Modals take over the whole screen.
	if m.errText != "" {
		return errModal(m.errText, m.w, m.h)
	}
	if m.form.active {
		return m.form.view(m.w, m.h)
	}
	if m.choice.active {
		return choiceModal(m.choice.title, m.choice.options, m.choice.cursor, m.w, m.h)
	}
	if m.modal {
		title, lines := m.modalTitle(), m.detailLines()
		if m.modalOverride != nil { // e.g. the task modal opened from the PRs tab
			title, lines = m.modalOverrideTitle, m.modalOverride
		}
		return modal(title, lines, m.detail, m.w, m.h)
	}
	top := tabStrip(labels, m.tab, m.w)
	var body string
	if m.tab == 1 && m.wide() { // bespoke: list + live tmux pane; right detail unless §-hidden
		body = m.agentsBody()
	} else if m.tab == 2 && m.wide() { // bespoke: list + diff/lint; right detail unless §-hidden
		body = m.prBody()
	} else if m.showDetail() {
		left := pane(rowTexts(m.rows()), m.list, m.leftWidth(), m.cursor[m.tab])
		right := pane(m.detailLines(), m.detail, m.detailWidth(), m.detailHighlight())
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, divider(m.bodyHeight()), right)
	} else {
		// Narrow terminal: selector full-width; detail is ENTER-only.
		body = pane(rowTexts(m.rows()), m.list, m.w, m.cursor[m.tab])
	}
	var foot string
	if m.mode != inputNone {
		foot = dimStyle.Render(padTrunc("enter submit · esc cancel", m.w)) + "\n" + m.input.View()
	} else {
		global := "⇥/⇧⇥ tab · C-h/l pane · § detail · j/k move · J/K scroll · r refresh · q quit"
		if m.flash != "" {
			global = m.flash
		}
		foot = footer(global, m.contextFooter(), m.w)
	}
	return strings.Join([]string{top, body, foot}, "\n")
}


