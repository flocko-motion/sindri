package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/flo-at/sindri/internal/api"
)

// TestGlyphsCountAsTerminalsDrawThem: every glyph the TUI places inside a width-counted cell
// must be counted at the width a terminal actually advances. A rune in an emoji block is
// drawn two cells wide; counted as one, its row is wider than the column, JoinHorizontal
// widens the whole block past the screen, every row wraps, and the frame grows taller than
// the terminal — which scrolls the top bar out of view.
func TestGlyphsCountAsTerminalsDrawThem(t *testing.T) {
	for _, g := range []string{eyeGlyph, warnGlyph} {
		if w := ansi.StringWidth(g); w != 2 {
			t.Errorf("%q counts as %d; an emoji-presentation glyph is drawn 2 cells wide "+
				"(add U+FE0F, or choose a narrow glyph)", g, w)
		}
	}
	// The narrow glyphs: genuinely one cell, and must stay that way.
	for _, g := range []string{"◉", "●", "✓", "◇", "«", "─", "│"} {
		if w := ansi.StringWidth(g); w != 1 {
			t.Errorf("%q counts as %d, expected a single cell", g, w)
		}
	}
}

// TestAgentRowFitsItsColumn is the invariant the bug broke: whatever a row contains, fitting
// it to the column must produce exactly that many cells — including for a dialed-into agent,
// whose row carries the eye marker.
func TestAgentRowFitsItsColumn(t *testing.T) {
	m := newModel(nil, nil, "/r/sindri")
	m.scopeRepo = false
	m.state = api.BoardState{
		Agents: []api.AgentView{
			{Name: "hepti", Role: "coauthor", Status: "collab", Task: "td-3", Clients: 1},
			{Name: "eitri", Role: "worker", Status: "working", Task: "td-4"},
		},
		Orphans: []string{"sindri-stale-abc-nabbi"},
	}
	for _, w := range []int{40, 60, 80, 132} {
		for _, r := range m.agentRows() {
			if got := ansi.StringWidth(padTrunc(r.text, w)); got != w {
				t.Errorf("w=%d row %q fits to %d cells", w, r.id, got)
			}
		}
	}
}

// TestAgentsBodyNeverExceedsWidth: the rendered body must fit the terminal exactly. One
// over-wide line makes JoinHorizontal pad every other line to match, so this catches the
// overflow at the level where it actually caused the vanishing header.
func TestAgentsBodyNeverExceedsWidth(t *testing.T) {
	for _, w := range []int{60, 80, 120} {
		m := newModel(nil, nil, "/r/sindri")
		m.tab = 1
		m.scopeRepo = false
		m.w, m.h = w, 30
		m.state = api.BoardState{
			Projects: []api.Project{{Tag: "sin", Path: "/r/sindri"}},
			Agents: []api.AgentView{
				{Name: "hepti", Role: "coauthor", Status: "collab", Task: "td-3", Clients: 2},
				{Name: "eitri", Role: "worker", Status: "working", Task: "td-4"},
			},
		}
		m.agentPane = "❯ working on it\n✻ Thinking…\n─────────────"
		m.reclamp()

		for i, line := range strings.Split(m.agentsBody(), "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("w=%d: body line %d is %d cells wide — it will wrap:\n%q", w, i, got, line)
			}
		}
	}
}
