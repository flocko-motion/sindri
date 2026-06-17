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
type errMsg struct{ err error }

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
	inputNewTask
	inputNewAgent
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

	detailKey  string
	agentLog   []store.Event
	prDetail   hub.PRDetail
	taskDetail store.Task
	quit       bool

	mode        inputMode      // active text-input modal
	input       textinput.Model
	inputTarget string         // selection captured when the modal opened
}

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

func (m model) Init() tea.Cmd { return waitForState(m.ch) }

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
	case stateMsg:
		m.state = hub.BoardState(msg)
		m.reclamp()
		return m, tea.Batch(waitForState(m.ch), m.syncDetail())
	case logMsg:
		m.agentLog = msg.evs
	case prMsg:
		m.prDetail = msg.d
	case taskMsg:
		m.taskDetail = msg.t
	case errMsg:
		m.err = msg.err
		return m, tea.Quit
	case tea.KeyMsg:
		if m.mode != inputNone {
			return m.updateInput(msg)
		}
		cmd := m.onKey(msg.String())
		if m.quit {
			return m, tea.Quit
		}
		return m, cmd
	}
	return m, nil
}

// updateInput routes a keypress to the open modal: esc cancels, enter submits,
// everything else edits the field.
func (m model) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode, m.inputTarget = inputNone, ""
		m.input.Blur()
		return m, nil
	case "enter":
		cmd := m.submitInput()
		m.mode, m.inputTarget = inputNone, ""
		m.input.Blur()
		return m, cmd
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// openInput starts a modal, capturing the current selection as its target.
func (m *model) openInput(mode inputMode, prompt string) {
	m.mode, m.inputTarget = mode, m.selID()
	m.input.SetValue("")
	m.input.Prompt = prompt
	m.input.Focus()
}

// submitInput performs the modal's hub action with the entered value.
func (m *model) submitInput() tea.Cmd {
	v := strings.TrimSpace(m.input.Value())
	if v == "" || m.cl == nil {
		return nil
	}
	cl, target := m.cl, m.inputTarget
	switch m.mode {
	case inputNewTask:
		return func() tea.Msg { _, _ = cl.NewTask(v, "", "", nil); return nil }
	case inputNewAgent:
		return func() tea.Msg { _ = cl.NewAgent(v, "worker"); return nil }
	case inputTell:
		return func() tea.Msg { _ = cl.Tell(target, v, "user"); return nil }
	}
	return nil
}

// onKey applies a key (by its string form) — shared by the live loop and the
// headless Screenshot harness. Mutates the model; returns an optional cmd.
func (m *model) onKey(k string) tea.Cmd {
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
		m.cursor[m.tab]++
	case "k", "up":
		m.cursor[m.tab]--
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
	case "h":
		if m.tab == 0 {
			if id := m.selID(); id != "" {
				m.collapsed[id] = true
			}
		}
	case "l":
		if m.tab == 0 { // tasks: expand fold
			delete(m.collapsed, m.selID())
		} else if m.tab == 1 { // agents: launch
			return m.action(func(id string) error { return m.cl.Launch(id, false) })
		}
	case "a": // agents: attach to the live tmux session (out-of-band)
		if m.tab == 1 {
			if id := m.selID(); id != "" && m.cl != nil {
				return tea.ExecProcess(attachCmd(id), func(error) tea.Msg { return nil })
			}
		}
	case "m": // prs: merge (the human gate)
		if m.tab == 2 {
			return m.action(func(id string) error { _, err := m.cl.Merge(id); return err })
		}
	case "n": // new task (tasks) / new agent (agents)
		if m.tab == 0 {
			m.openInput(inputNewTask, "new task: ")
			return textinput.Blink
		} else if m.tab == 1 {
			m.openInput(inputNewAgent, "new agent name: ")
			return textinput.Blink
		}
	case "t": // tell the selected agent
		if m.tab == 1 && m.selID() != "" {
			m.openInput(inputTell, "tell "+m.selID()+": ")
			return textinput.Blink
		}
	case "r":
		cl := m.cl
		m.reclamp()
		return func() tea.Msg { cl.Refresh(); return nil }
	}
	m.reclamp()
	return m.syncDetail()
}

// action runs a mutating hub call for the current selection; /events then
// refreshes the view.
func (m *model) action(fn func(id string) error) tea.Cmd {
	id := m.selID()
	if id == "" || m.cl == nil {
		return nil
	}
	return func() tea.Msg { _ = fn(id); return nil }
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
	m.list.SetHeight(m.bodyHeight())
	m.list.SetTotal(n)
	m.list.SetCursor(m.cursor[m.tab])
	m.detail.SetHeight(m.bodyHeight())
	m.detail.SetTotal(len(m.detailLines()))
	m.detail.ScrollTop()
}

// syncDetail fetches the selected item's rich detail when the selection changes.
func (m *model) syncDetail() tea.Cmd {
	key := fmt.Sprintf("%d:%s", m.tab, m.selID())
	if key == m.detailKey || m.cl == nil {
		return nil
	}
	m.detailKey = key
	id := m.selID()
	if id == "" {
		return nil
	}
	cl := m.cl
	switch m.tab {
	case 0:
		return func() tea.Msg { t, _ := cl.TaskInfo(id); return taskMsg{id, t} }
	case 1:
		return func() tea.Msg { evs, _ := cl.Log(id); return logMsg{id, evs} }
	default:
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
		return fmt.Sprintf("n new · f filter: %s · h/l fold", filterNames[m.filter])
	case 1:
		return "n new · l launch · t tell · a attach"
	default:
		return "m merge"
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
	top := tabStrip(labels, m.tab, m.w)
	left := pane(rowTexts(m.rows()), m.list, m.leftWidth(), m.cursor[m.tab])
	right := pane(m.detailLines(), m.detail, m.detailWidth(), -1)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, divider(m.bodyHeight()), right)
	var foot string
	if m.mode != inputNone {
		foot = dimStyle.Render(padTrunc("enter submit · esc cancel", m.w)) + "\n" + m.input.View()
	} else {
		foot = footer("ctrl+h/l tab · j/k move · g/G ends · r refresh · q quit", m.contextFooter(), m.w)
	}
	return strings.Join([]string{top, body, foot}, "\n")
}

func rowTexts(rows []row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.text
	}
	return out
}

// row is one selector line: display text + the id it selects ("" = not selectable).
type row struct {
	text string
	id   string
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Screenshot renders the dashboard headlessly at w×h with the given board state
// and a sequence of key presses applied in order — for verifying layout without
// a terminal or a hub. Returned cmds are not run (no hub calls), so detail panes
// show their synchronous content only.
//
//deadcode:keep — dev/test harness for headless rendering
func Screenshot(st hub.BoardState, w, h int, keys ...string) string {
	m := newModel(nil, nil)
	m.w, m.h = w, h
	m.state = st
	m.reclamp()
	for _, k := range keys {
		m.onKey(k)
	}
	return m.View()
}
