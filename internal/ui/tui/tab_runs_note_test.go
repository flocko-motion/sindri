package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/flo-at/sindri/internal/api"
)

// runsBoard is a board on the Runs tab, with n runs in the selected repo.
func runsBoard(n int) api.BoardState {
	b := api.BoardState{Projects: []api.Project{{Tag: "sin", Path: "/r/sindri"}}}
	for i := 0; i < n; i++ {
		b.Runs = append(b.Runs, api.Run{
			Project: "sin", ID: "run-" + string(rune('a'+i)), Agent: "eitri",
			Command: "make verify", Status: "queued", CreatedAt: "2026-08-14T10:00:00Z",
		})
	}
	return b
}

// runsScreen renders the Runs tab at a size, with the tab already selected.
func runsScreen(t *testing.T, b api.BoardState, w, h int) string {
	t.Helper()
	// Five tabs forward from Tasks — the Runs tab is last (-> tuiSections).
	return ansi.Strip(Screenshot(b, w, h, "tab", "tab", "tab", "tab", "tab"))
}

// TestTheRunsTabAlwaysExplainsItself: the tab is the least self-evident of the lot, and the
// surprising part — one run at a time across every repo — is what "why is mine not starting" asks,
// a question that arrives when the list is FULL. So the line stays whether or not there are rows.
func TestTheRunsTabAlwaysExplainsItself(t *testing.T) {
	for _, n := range []int{0, 3} {
		got := runsScreen(t, runsBoard(n), 160, 24)
		for _, want := range []string{"A run is a command", "ONE at a time across every repo", "Press N to queue one", "agents queue their own"} {
			if !strings.Contains(got, want) {
				t.Errorf("%d run(s): the note should carry %q:\n%s", n, want, got)
			}
		}
	}
}

// TestTheNoteWrapsRatherThanTruncates: a half-sentence explains nothing, so a narrow terminal gets
// a second line rather than an ellipsis — and never more than two.
func TestTheNoteWrapsRatherThanTruncates(t *testing.T) {
	m := newModel(nil, nil, "")
	m.state = runsBoard(0)
	for _, w := range []int{80, 100, 160, 200} {
		m.w = w
		lines := m.runsNote(w)
		if len(lines) > 2 {
			t.Errorf("width %d: the note took %d lines, want at most 2: %q", w, len(lines), lines)
		}
		joined := ansi.Strip(strings.Join(lines, " "))
		if !strings.Contains(joined, "agents queue their own") {
			t.Errorf("width %d: the note was cut short: %q", w, joined)
		}
	}
	// One line where there is room for one.
	m.w = 200
	if got := len(m.runsNote(200)); got != 1 {
		t.Errorf("a wide terminal should take one line, got %d", got)
	}
}

// TestTheNoteCostsRowsRatherThanTheScreen: the rows must survive the note. On a short terminal it
// takes lines from the list rather than pushing runs out of view entirely.
func TestTheNoteCostsRowsRatherThanTheScreen(t *testing.T) {
	b := runsBoard(3)
	got := runsScreen(t, b, 160, 10)
	if !strings.Contains(got, "run-a") {
		t.Errorf("the rows must still be visible under the note:\n%s", got)
	}
	// And the pane never collapses below a row, however short the terminal.
	m := newModel(nil, nil, "")
	m.state, m.w, m.h = b, 80, 1
	if h := m.runsPaneHeight(); h < 1 {
		t.Errorf("run pane height = %d on a tiny terminal, want at least 1", h)
	}
}

// TestTheEmptyRunsListSaysSo: the note explains what runs ARE; it does not say whether there are
// any. Without its own empty state, a tab with nothing queued reads as one that failed to load.
func TestTheEmptyRunsListSaysSo(t *testing.T) {
	got := runsScreen(t, runsBoard(0), 160, 24)
	if !strings.Contains(got, "(no runs)") {
		t.Errorf("an empty Runs tab should say so where the rows would be:\n%s", got)
	}
	if strings.Contains(runsScreen(t, runsBoard(2), 160, 24), "(no runs)") {
		t.Error("the empty state must not show with runs on the board")
	}
}
