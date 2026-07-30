package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/hub/store"
	"github.com/muesli/termenv"
)

// heptiBoard reproduces the board from the report: three agents in one repo, two orphan pods,
// and a coauthor (hepti) whose workspace is the repo root itself.
func heptiBoard() hub.BoardState {
	return hub.BoardState{
		Projects: []store.Project{{Tag: "rdb", Path: "/r/ranke-db"}},
		Agents: []hub.AgentView{
			{Project: "rdb", Repo: "ranke-db", Name: "galar", Role: "planner", Status: "planning", Task: "os-new"},
			{Project: "rdb", Repo: "ranke-db", Name: "hepti", Role: "coauthor", Status: "collab", Workspace: ".", Memory: "2g", Container: "sindri-ranke-db-984c491b-hepti"},
			{Project: "rdb", Repo: "ranke-db", Name: "nori", Role: "worker", Status: "idle", Task: "os-adc678"},
		},
		Orphans: []string{"sindri-ranke-db-68404287-brokkr", "sindri-ranke-graph-9ec79211-eitri"},
	}
}

// TestFrameIsExactlyTerminalHeight is the invariant behind the vanishing top bar: the rendered
// frame must be exactly as tall as the terminal. One line too many and the terminal scrolls,
// taking the header row with it — the content appears to shift up by one and the tab bar is gone.
//
// The cursor is placed on each agent in turn, because the report is cursor-dependent.
func TestFrameIsExactlyTerminalHeight(t *testing.T) {
	for _, w := range []int{80, 100, 120} {
		for cursor := 0; cursor < 5; cursor++ {
			m := newModel(nil, nil, "/r/ranke-db")
			m.tab = 1 // Agents
			m.w, m.h = w, 24
			m.state = heptiBoard()
			m.cursor[1] = cursor
			m.reclamp()

			lines := strings.Split(m.View(), "\n")
			if len(lines) != m.h {
				t.Errorf("w=%d cursor=%d: frame is %d lines, terminal is %d", w, cursor, len(lines), m.h)
			}
			for i, l := range lines {
				if got := lipgloss.Width(l); got > m.w {
					t.Errorf("w=%d cursor=%d: line %d is %d cells wide:\n%q", w, cursor, i, got, l)
				}
			}
		}
	}
}

// TestFrameHeightWithMultilineLogEntry is the reported bug. An agent's activity log carries
// chat messages verbatim, and those are multi-line — the real one that triggered this had 123
// newlines in a single entry. Rendered into a cell that counts it as one line, every newline
// added a row, pushing the frame past the terminal so the top bar scrolled away.
func TestFrameHeightWithMultilineLogEntry(t *testing.T) {
	// Colour must be ON, or this test cannot see the bug. lipgloss emits no escapes when
	// stdout is not a terminal, and without them sanitize takes its plain path, whose
	// control-character filter already drops newlines. The real TUI styles the timestamp, so
	// the string carries ANSI, sanitize returned early, and the newlines survived to the frame.
	restore := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(restore) })

	m := newModel(nil, nil, "/r/ranke-db")
	m.tab = 1
	m.w, m.h = 100, 24
	m.state = heptiBoard()
	m.cursor[1] = 1 // hepti, the coauthor in the meeting room
	m.agentLog = []store.Event{
		{TS: "2026-07-28T09:02:25Z", Type: "chat", Payload: "[chat] alviss: one\ntwo\nthree\nfour"},
		{TS: "2026-07-28T09:04:27Z", Type: "chat", Payload: "[chat] alviss: " + strings.Repeat("line\n", 123)},
		{TS: "2026-07-28T12:29:51Z", Type: "launch", Payload: "requested"},
	}
	m.reclamp()

	lines := strings.Split(m.View(), "\n")
	if len(lines) != m.h {
		t.Errorf("frame is %d lines, terminal is %d — a multi-line log entry must occupy one row", len(lines), m.h)
	}
	for i, l := range lines {
		if got := lipgloss.Width(l); got > m.w {
			t.Errorf("line %d is %d cells wide (max %d)", i, got, m.w)
		}
	}
}

// TestSanitizeFoldsNewlines pins the choke point: a cell is one line, whether or not the text
// carries styling. The styled case is the one that leaked — it took an early return.
func TestSanitizeFoldsNewlines(t *testing.T) {
	for _, s := range []string{
		"plain\nsecond",
		"\x1b[2m09:02\x1b[0m  chat  [chat] alviss: one\ntwo",
		"crlf\r\nsecond",
		"bare\rreturn",
	} {
		if got := padTrunc(s, 40); strings.ContainsAny(got, "\n\r") {
			t.Errorf("padTrunc kept a line break: %q -> %q", s, got)
		}
	}
}

// TestFrameHeightWithLivePane: the agent pane holds another program's screen — box drawing,
// long rows, more lines than the region. None of that may change the frame's height.
func TestFrameHeightWithLivePane(t *testing.T) {
	pane := strings.Join([]string{
		"  ├──────────────────────────────────────┼──────────────────────────────┤",
		"  │ encoding: json-seq | cbor-seq        │ ResultJSON/ResultCBOR (frame) │",
		"  └──────────────────────────────────────┴──────────────────────────────┘",
		strings.Repeat("wide ", 60),
	}, "\n")
	for _, h := range []int{18, 24, 40} {
		m := newModel(nil, nil, "/r/ranke-db")
		m.tab = 1
		m.w, m.h = 80, h
		m.state = heptiBoard()
		m.cursor[1] = 1 // hepti
		m.agentPane = pane
		m.reclamp()

		if lines := strings.Split(m.View(), "\n"); len(lines) != h {
			t.Errorf("h=%d: frame is %d lines with a live pane", h, len(lines))
		}
	}
}

// TestFrameTestsActuallyRenderTheDetail guards the tests above: they exist to catch content in
// the RIGHT column, so a width where the column is hidden would make them pass vacuously.
func TestFrameTestsActuallyRenderTheDetail(t *testing.T) {
	m := newModel(nil, nil, "/r/ranke-db")
	m.tab = 1
	m.w, m.h = 100, 24
	if !m.showDetail() {
		t.Fatalf("at w=%d the detail column is hidden (detailMinWidth); the frame tests would not exercise it", m.w)
	}
}

// TestPaneLineTerminatesStyle: a captured pane line is coloured with SGR and is wider than
// our cell, so it gets truncated. If the truncation drops the reset, the attribute survives
// to the end of the frame and the terminal carries it into the NEXT frame — whose first row
// is the header.
func TestPaneLineTerminatesStyle(t *testing.T) {
	// A realistic line: colour opened early, reset only at the far right.
	line := "\x1b[36m" + strings.Repeat("x", 100) + " tail\x1b[0m"
	got := padTrunc(line, 40)
	t.Logf("padTrunc(40) = %q", got)
	if strings.Contains(got, "\x1b[") && !strings.HasSuffix(got, "\x1b[0m") && !strings.HasSuffix(got, "\x1b[m") {
		t.Errorf("truncated coloured line leaves the style open: %q", got)
	}
}
