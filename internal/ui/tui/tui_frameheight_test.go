package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/api"
)

// everyTabBoard gives every tab something to render, so a frame is measured with content in it
// rather than with an empty pane that would fit anything.
func everyTabBoard() api.BoardState {
	b := heptiBoard()
	b.Tasks = []api.Task{
		{ID: "td-1", Title: "Wire the login form", Status: "open", Priority: "P1", Type: "task"},
		{ID: "td-2", Title: "Session handling", Status: "in_progress", Priority: "P2", Type: "task"},
	}
	b.PRs = []api.PR{{ID: "pr-1", Project: "rdb", Task: "td-2", Agent: "nori", Branch: "td-2", Base: "main", Status: "open"}}
	b.Runs = []api.Run{
		{ID: "run-1", Project: "rdb", Agent: "nori", Command: "make verify", Status: "running"},
		{ID: "run-2", Project: "rdb", Agent: api.SenderUser, Command: "go test ./...", Status: "queued", Position: 1},
	}
	b.Mail = []api.Mail{{ID: 1, Project: "rdb", Agent: "nori", Sender: "user", Body: "read the brief again"}}
	b.MailTotal, b.MailUnread = 1, 1
	b.Chat = api.ChatView{Members: []api.ChatMember{{Project: "rdb", Name: "hepti", Role: "coauthor"}}}
	return b
}

// TestEveryTabFitsTheTerminal is the height contract, over every tab rather than the one that broke
// last. A frame one line too tall scrolls the terminal, and what scrolls off the top is the header —
// the tab strip, the repo name and the memory badge — so the tab reads as having replaced it.
//
// Each tab honours the contract by its own arithmetic, and the Runs tab composes its body by hand
// instead of through pane(), which is why it was the one that got it wrong. Nothing checked.
func TestEveryTabFitsTheTerminal(t *testing.T) {
	for _, size := range []struct{ w, h int }{{100, 24}, {150, 40}, {70, 16}} {
		for _, hidden := range []bool{false, true} {
			for tab := range tuiSections {
				m := newModel(nil, nil, "/r/ranke-db")
				m.tab = tab
				m.w, m.h = size.w, size.h
				m.hideDetail = hidden
				m.state = everyTabBoard()
				m.reclamp()

				lines := strings.Split(m.View(), "\n")
				if len(lines) != m.h {
					t.Errorf("%s tab at %dx%d (detail hidden=%v): frame is %d lines, terminal is %d",
						tuiSections[tab].Title, size.w, size.h, hidden, len(lines), m.h)
				}
				for i, l := range lines {
					if got := lipgloss.Width(l); got > m.w {
						t.Errorf("%s tab at %dx%d (detail hidden=%v): line %d is %d cells wide",
							tuiSections[tab].Title, size.w, size.h, hidden, i, got)
					}
				}
			}
		}
	}
}

// TestTheRunsDetailPaneFillsItsSlot: the note takes its lines out of the panes below it, so the
// detail viewport is shorter than the body. Sized to the body instead, it padded itself past its
// slot — the frame overflowed by exactly the note's height — and J/K under-scrolled, believing there
// was more room at the bottom than the screen gives.
func TestTheRunsDetailPaneFillsItsSlot(t *testing.T) {
	m := newModel(nil, nil, "/r/ranke-db")
	m.tab = 5 // Runs
	m.w, m.h = 100, 24
	m.state = everyTabBoard()
	m.reclamp()

	if len(m.runsNote(m.w)) == 0 {
		t.Fatal("the note is what makes this tab's panes shorter than the body; it rendered nothing")
	}
	if got, want := m.detail.Height, m.runsPaneHeight(); got != want {
		t.Errorf("the detail pane is %d lines but its slot is %d — the note's lines must come out of it", got, want)
	}
	if m.runsPaneHeight() >= m.bodyHeight() {
		t.Fatal("the note cost the panes nothing, so this test proves nothing about the slot")
	}
}

// TestTheRunsDetailScrollsToItsEnd: the pane renders the WRAPPED detail, so a viewport told the
// unwrapped count stops short of the last line and the tail of a run's output is unreachable.
func TestTheRunsDetailScrollsToItsEnd(t *testing.T) {
	m := newModel(nil, nil, "/r/ranke-db")
	m.tab = 5
	m.w, m.h = 100, 24
	m.state = everyTabBoard()
	// A stored output whose lines are far wider than the column, so wrapping multiplies them.
	m.runDetail = api.RunDetail{
		Run:    api.Run{ID: "run-1", Project: "rdb", Agent: "nori", Command: "make verify", Status: "running"},
		Output: strings.Repeat(strings.Repeat("wide output ", 30)+"\n", 6),
	}
	m.reclamp()

	wrapped, _ := wrapContentMapped(m.detailLines(), m.detailWidth())
	if len(wrapped) <= len(m.detailLines()) {
		t.Fatal("the fixture must wrap, or the unwrapped count would be right by accident")
	}
	if got := m.detail.Total; got != len(wrapped) {
		t.Errorf("the detail viewport counts %d lines; the pane renders %d wrapped ones", got, len(wrapped))
	}
}
