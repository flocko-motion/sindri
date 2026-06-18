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
	"os/exec"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/adapter/pod"
	"github.com/flo-at/sindri/internal/adapter/tmux"
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
	_, err = tea.NewProgram(newModel(cl, ch), tea.WithAltScreen()).Run()
	return err
}

type stateMsg hub.BoardState
type logMsg struct {
	key string
	evs []store.Event
}
type prMsg struct {
	key string
	d   hub.PRDetail
}
type taskMsg struct {
	key string
	t   store.Task
}
type paneMsg struct {
	agent string
	text  string
}
type prLintMsg struct {
	pr   string
	text string
}
type reviewPromptMsg string

// paneLines is how many rows of an agent's tmux scrollback the detail shows.
const paneLines = 200
type errMsg struct{ err error }       // fatal: hub connection lost
type errModalMsg struct{ err error }  // non-fatal: show the error modal

// tickMsg drives periodic polling; polledMsg carries a state fetched by a poll
// (distinct from stateMsg so it doesn't re-arm the SSE waiter).
type tickMsg time.Time
type polledMsg hub.BoardState

const refreshInterval = 3 * time.Second

// detailScrollStep is how many lines J/K scroll the detail pane at once.
const detailScrollStep = 5

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
	state hub.BoardState
	err   error
	w, h  int

	tab    int
	cursor [3]int
	list   scroll.Viewport
	detail scroll.Viewport

	filter    int
	collapsed map[string]bool

	rightFocus  bool // detail (right) column has focus (h/l switch; j/k move within)
	rightCursor int  // focused actionable item in the right column

	detailKey  string
	agentLog   []store.Event
	agentPane  string // captured tmux screen of the selected agent (live)
	prDetail     hub.PRDetail
	prLint       string // lint output for the selected PR (shown in the big pane)
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

func (m model) showDetail() bool { return m.w >= detailMinWidth }

func newModel(cl *client.HTTP, ch <-chan hub.BoardState) model {
	// Default to a sane size so a frame renders immediately — the real size
	// arrives via WindowSizeMsg and resizes. (Some terminals send the initial
	// size late or as 0×0; without a default the view would stick on "loading".)
	in := textinput.New()
	in.CharLimit = 200
	m := model{cl: cl, ch: ch, collapsed: map[string]bool{}, w: 80, h: 24, input: in}
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
	case prLintMsg:
		if msg.pr == m.selID() {
			m.prLint = msg.text
			m.detail.Resize(m.detail.Height, len(m.prContentLines())) // re-clamp for the new content
		}
	case reviewPromptMsg:
		m.reviewPrompt = string(msg)
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
			if act := m.prActionable(); m.rightCursor < len(act) {
				_ = clipboard.WriteAll(act[m.rightCursor].value)
				m.flash = "copied: " + act[m.rightCursor].value
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
	case "ctrl+l", "tab":
		m.tab = (m.tab + 1) % len(hub.Sections)
	case "ctrl+h", "shift+tab":
		m.tab = (m.tab - 1 + len(hub.Sections)) % len(hub.Sections)
	case "1", "2", "3":
		m.tab = int(k[0] - '1')
	case "j", "down":
		if m.rightFocus {
			m.rightCursor = clampInt(m.rightCursor+1, 0, max(0, len(m.prActionable())-1))
		} else {
			m.cursor[m.tab]++
		}
	case "k", "up":
		if m.rightFocus {
			m.rightCursor = clampInt(m.rightCursor-1, 0, max(0, len(m.prActionable())-1))
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
	case "g":
		m.cursor[m.tab] = 0
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
	case "h": // tasks: collapse fold · prs: focus the list (left)
		if m.tab == 0 {
			if id := m.selID(); id != "" {
				m.collapsed[id] = true
			}
		} else if m.tab == 2 {
			m.rightFocus = false
		}
	case "l": // tasks: expand fold · prs: focus the detail column (right)
		if m.tab == 0 {
			delete(m.collapsed, m.selID())
		} else if m.tab == 2 && m.showDetail() {
			m.rightFocus = true
			m.rightCursor = clampInt(m.rightCursor, 0, max(0, len(m.prActionable())-1))
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
					return tea.ExecProcess(attachCmd(a.Name), func(error) tea.Msg { return nil })
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
		} else if m.tab == 1 { // agents: auto-named after a dwarf
			cl := m.cl
			m.flash = "registering a new agent…"
			return mutateThenRefresh(cl, func() { _, _ = cl.NewAgent("", "worker") })
		}
	case "e": // edit the selected task (tasks) / set role (agents)
		if m.tab == 0 && m.selID() != "" {
			m.openTaskForm(true)
			return nil
		} else if m.tab == 1 && m.selID() != "" {
			m.openRoleChoice(m.selID())
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
	case "R": // prs: reject with a (multiline) reason
		if m.tab == 2 && m.selID() != "" {
			m.openRejectForm(m.selID())
			return nil
		}
	case "A": // prs: request an agentic review (editable instruction)
		if m.tab == 2 && m.selID() != "" {
			m.openReviewForm(m.selID())
			return nil
		}
	case "p": // set the selected task's priority
		if m.tab == 0 && m.selID() != "" {
			m.openPriorityChoice(m.selID())
			return nil
		}
	case "enter":
		if m.rightFocus { // act on the focused right-column item (jump/open)
			return m.activateRightItem()
		}
		if m.selID() != "" { // open the full-screen detail modal
			m.modal = true
			m.detail.SetHeight(modalContentHeight(m.h))
			m.detail.SetTotal(len(m.detailLines()))
			m.detail.ScrollTop()
			return nil
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

// refreshCmd asks the hub to re-sync tasks from the source of truth.
func (m *model) refreshCmd() tea.Cmd {
	cl := m.cl
	if cl == nil {
		return nil
	}
	return func() tea.Msg { cl.Refresh(); return nil }
}

// action runs a mutating hub call for the current selection; /events then
// refreshes the view.
func (m *model) action(fn func(id string) error) tea.Cmd {
	id := m.selID()
	if id == "" || m.cl == nil {
		return nil
	}
	cl := m.cl
	return func() tea.Msg {
		if err := fn(id); err != nil {
			return errModalMsg{err} // surface failures instead of swallowing them
		}
		st, err := cl.State() // reflect the change at once
		if err != nil {
			return nil
		}
		return polledMsg(st)
	}
}

// attachCmd builds the interactive `podman exec -it … tmux attach` for an agent.
func attachCmd(name string) *exec.Cmd {
	args := append([]string{"exec", "-it", hub.Container(name), "tmux"}, tmux.Attach(name, false)...)
	return exec.Command(pod.Binary, args...)
}

// reclamp keeps the active tab's cursor + both viewports in range.
func (m *model) reclamp() {
	n := len(m.rows())
	m.cursor[m.tab] = clampInt(m.cursor[m.tab], 0, max(0, n-1))
	listH := m.bodyHeight()
	if m.showDetail() { // agents/prs: the list is the short top region of a split
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
	if m.tab == 2 && m.showDetail() { // PRs: detail pane is the big bottom-left content
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
		m.agentPane = "" // selection changed — drop the previous agent's screen
		return tea.Batch(
			func() tea.Msg { evs, _ := cl.Log(id); return logMsg{id, evs} },
			paneFetchCmd(cl, id),
		)
	default:
		m.prLint = "" // new PR → show its diff, not the previous lint
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
	switch m.tab {
	case 0:
		return fmt.Sprintf("N new · e edit · p priority · y/Y yank · f filter: %s · h/l fold", filterNames[m.filter])
	case 1:
		return "N new · S start/stop · t tell · a attach · e role · D delete"
	default:
		if m.rightFocus {
			return "j/k item · enter open · y copy · h back to list"
		}
		return "l focus detail · t task · A review · R reject · L lint · m merge"
	}
}

func (m model) selID() string {
	r := m.rows()
	if c := m.cursor[m.tab]; c >= 0 && c < len(r) {
		return r[c].id
	}
	return ""
}

// selectRow moves the current tab's cursor to the row with the given id.
func (m *model) selectRow(id string) {
	for i, r := range m.rows() {
		if r.id == id {
			m.cursor[m.tab] = i
			return
		}
	}
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
	if m.tab == 1 && m.showDetail() {
		body = m.agentsBody() // bespoke: list + live tmux pane (left) · detail (right)
	} else if m.tab == 2 && m.showDetail() {
		body = m.prBody() // bespoke: list + diff/lint (left) · metadata+task+reviews (right)
	} else if m.showDetail() {
		left := pane(rowTexts(m.rows()), m.list, m.leftWidth(), m.cursor[m.tab])
		right := pane(m.detailLines(), m.detail, m.detailWidth(), -1)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, divider(m.bodyHeight()), right)
	} else {
		// Narrow terminal: selector full-width; detail is ENTER-only.
		body = pane(rowTexts(m.rows()), m.list, m.w, m.cursor[m.tab])
	}
	var foot string
	if m.mode != inputNone {
		foot = dimStyle.Render(padTrunc("enter submit · esc cancel", m.w)) + "\n" + m.input.View()
	} else {
		global := "ctrl+h/l tab · j/k move · J/K scroll · g/G ends · r refresh · q quit"
		if m.flash != "" {
			global = m.flash
		}
		foot = footer(global, m.contextFooter(), m.w)
	}
	return strings.Join([]string{top, body, foot}, "\n")
}


