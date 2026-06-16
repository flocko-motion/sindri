// package: tui / tui
// type:    ui (a thin hub client)
// job:     a lean Bubble Tea dashboard that renders the hub's board (/state) and
//          live-updates over /events — agents with their workflow phase/task/PR,
//          open tasks, merge-intents, and orphan warnings. Enter shows an agent's
//          activity timeline (/log). It owns no domain logic and reaches nothing
//          but the hub.
// limits:  refuses to start without a running hub; all data comes from the hub.
package tui

import (
	"context"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
)

// Run starts the TUI against the repo's hub. It refuses to start without one
// (the hub is the single source of truth — D-hub / task 4.3).
func Run(root string) error {
	if !hub.IsRunning(root) {
		return fmt.Errorf("no hub running — start one first: 'sindri hub &'")
	}
	cl := client.Dial(root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := cl.Watch(ctx)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(model{cl: cl, ch: ch}, tea.WithAltScreen()).Run()
	return err
}

type stateMsg hub.BoardState
type logMsg []store.Event
type errMsg struct{ err error }

type model struct {
	cl       *client.HTTP
	ch       <-chan hub.BoardState
	state    hub.BoardState
	cursor   int
	logFor   string       // agent whose timeline is shown ("" = board)
	log      []store.Event
	err      error
	w, h     int
}

func (m model) Init() tea.Cmd { return waitForState(m.ch) }

// waitForState blocks on the next board snapshot from the SSE stream.
func waitForState(ch <-chan hub.BoardState) tea.Cmd {
	return func() tea.Msg {
		st, ok := <-ch
		if !ok {
			return errMsg{io.EOF}
		}
		return stateMsg(st)
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case stateMsg:
		m.state = hub.BoardState(msg)
		if m.cursor >= len(m.state.Agents) {
			m.cursor = max(0, len(m.state.Agents)-1)
		}
		return m, waitForState(m.ch) // keep listening
	case logMsg:
		m.log = []store.Event(msg)
	case errMsg:
		m.err = msg.err
		return m, tea.Quit
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.logFor = ""
	case "j", "down":
		if m.cursor < len(m.state.Agents)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "r":
		return m, func() tea.Msg { m.cl.Refresh(); return nil }
	case "enter":
		if m.cursor < len(m.state.Agents) {
			name := m.state.Agents[m.cursor].Name
			m.logFor = name
			return m, func() tea.Msg { evs, _ := m.cl.Log(name); return logMsg(evs) }
		}
	}
	return m, nil
}

var (
	head = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dim  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	warn = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	sel  = lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("238"))
)

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("hub connection lost: %v\n", m.err)
	}
	if m.logFor != "" {
		return m.logView()
	}
	var b strings.Builder
	fmt.Fprintln(&b, head.Render("Sindri — agents"))
	for i, a := range m.state.Agents {
		run := "·"
		if a.Running {
			run = "●"
		}
		row := fmt.Sprintf("%s %-12s %-8s %-9s %-10s %s", run, a.Name, a.Role, a.Phase, dashTUI(a.Task), dashTUI(a.PR))
		if i == m.cursor {
			row = sel.Render("▸ " + row)
		} else {
			row = "  " + row
		}
		fmt.Fprintln(&b, row)
	}
	for _, o := range m.state.Orphans {
		fmt.Fprintln(&b, warn.Render(fmt.Sprintf("  ⚠ orphan: %s (no roster entry — 'podman rm -f %s')", o, o)))
	}

	fmt.Fprintln(&b, head.Render("\nPRs"))
	if len(m.state.PRs) == 0 {
		fmt.Fprintln(&b, dim.Render("  none"))
	}
	for _, p := range m.state.PRs {
		fmt.Fprintf(&b, "  %-14s %-9s %-10s %s\n", p.ID, p.Status, p.Agent, p.Branch)
	}

	fmt.Fprintln(&b, head.Render("\nOpen tasks"))
	if len(m.state.Tasks) == 0 {
		fmt.Fprintln(&b, dim.Render("  none"))
	}
	for _, t := range m.state.Tasks {
		fmt.Fprintf(&b, "  %-12s %-8s %s\n", t.ID, t.Priority, t.Title)
	}

	fmt.Fprint(&b, dim.Render("\nj/k move · enter timeline · r refresh · q quit"))
	return b.String()
}

func (m model) logView() string {
	var b strings.Builder
	fmt.Fprintln(&b, head.Render("Timeline — "+m.logFor))
	if len(m.log) == 0 {
		fmt.Fprintln(&b, dim.Render("  (no activity)"))
	}
	for _, e := range m.log {
		fmt.Fprintf(&b, "  %s  %-10s %s\n", dim.Render(shortTS(e.TS)), e.Type, e.Payload)
	}
	fmt.Fprint(&b, dim.Render("\nesc back · q quit"))
	return b.String()
}

func dashTUI(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// shortTS trims an RFC3339 timestamp to HH:MM:SS for compact display.
func shortTS(ts string) string {
	if len(ts) >= 19 {
		return ts[11:19]
	}
	return ts
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
