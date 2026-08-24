package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/flo-at/sindri/internal/api"
)

// TestMenuEntryStyleIsNotDim is the contrast the task requires: with the menu no longer filling
// the screen, styling is the only thing left saying it is open.
func TestMenuEntryStyleIsNotDim(t *testing.T) {
	if sameStyle(menuEntryStyle, dimStyle) {
		t.Error("menu entries must not read the same as the ordinary dim hints")
	}
	if sameStyle(stWarn, menuEntryStyle) {
		t.Error("a hotkey must not read the same colour as its label")
	}
}

// TestMenuOpensInTheFooterNotOverTheScreen: the rows being acted on must still be on screen once
// the menu is open — the whole point of moving it out of a centre-screen overlay.
func TestMenuOpensInTheFooterNotOverTheScreen(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab, m.w, m.h = 0, 120, 30
	m.state = api.BoardState{Tasks: []api.Task{
		{ID: "td-1", Title: "a very findable title", Status: "open"},
	}}
	m.reclamp()
	m.onKey(keyMenu)
	if !m.menu {
		t.Fatal("precondition: space should open the menu")
	}

	view := m.View()
	if !strings.Contains(view, "a very findable title") {
		t.Errorf("the row being acted on should still be on screen while the menu is open:\n%s", view)
	}
	if want := m.menuFooter(m.w); !strings.HasSuffix(view, want) {
		t.Errorf("the frame's last two rows should be the menu, got:\n%s\nwant suffix:\n%s", view, want)
	}
}

// TestMenuNeverAddsAThirdRow is the one hard rule: the frame is top+body+foot joined to the
// terminal height, so a footer that grows pushes the header off the top (sd-882347, for Runs).
func TestMenuNeverAddsAThirdRow(t *testing.T) {
	for _, size := range []struct{ w, h int }{{100, 24}, {150, 40}, {60, 16}} {
		for tab := range tuiSections {
			m := newModel(nil, nil, "/r/ranke-db")
			m.tab, m.w, m.h = tab, size.w, size.h
			m.state = everyTabBoard()
			m.reclamp()
			m.menu = true

			lines := strings.Split(m.View(), "\n")
			if len(lines) != m.h {
				t.Errorf("%s tab at %dx%d with the menu open: frame is %d lines, terminal is %d",
					tuiSections[tab].Title, size.w, size.h, len(lines), m.h)
			}
			for i, l := range lines {
				if got := ansi.StringWidth(l); got > m.w {
					t.Errorf("%s tab at %dx%d: line %d is %d cells wide", tuiSections[tab].Title, size.w, size.h, i, got)
				}
			}
		}
	}
}

// TestMenuFooterAlwaysHasTwoRows: whatever a row offers, menuFooter never grows or shrinks past
// the footer's fixed two-row budget.
func TestMenuFooterAlwaysHasTwoRows(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab = 6
	m.state = api.BoardState{Mail: []api.Mail{{ID: 1, Agent: "nori", Sender: "hub", Body: "hi"}}}
	m.reclamp()

	rows := strings.Split(m.menuFooter(40), "\n")
	if len(rows) != 2 {
		t.Fatalf("menuFooter should always be exactly two rows, got %d", len(rows))
	}
}

// TestGlobalConfigFillsEveryTabsMenu documents a side effect of moving `E config` behind the
// prefix (sd-6d0ff2): it is scopeGlobal, shown on every tab, so once it commits there is always
// at least one offer — "nothing to commit on this row" stays correct code but is no longer
// reachable through any real tab, since Mail (the scope with no bindings of its own) still gets it.
func TestGlobalConfigFillsEveryTabsMenu(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab = 6 // Mail: no tab-local committing binding of its own
	m.state = api.BoardState{Mail: []api.Mail{{ID: 1, Agent: "nori", Sender: "hub", Body: "hi"}}}
	m.reclamp()

	offers := m.menuOffers()
	if len(offers) != 1 || offers[0].keys != keyConfig {
		t.Fatalf("Mail's own scope commits nothing; expected only the global config entry, got %v", offers)
	}
	got := ansi.Strip(m.menuFooter(40))
	if strings.Contains(got, "nothing to commit") {
		t.Errorf("the global config entry should have filled the menu, got %q", got)
	}
	if !strings.Contains(got, "config") {
		t.Errorf("expected the global config entry, got %q", got)
	}
}

// TestMenuFooterRowsAreExactlyWidth: the footer's two rows are a fixed-width component the frame
// composes by joining lines — a row short of width would misalign whatever sits below it.
func TestMenuFooterRowsAreExactlyWidth(t *testing.T) {
	m := prTabWith(api.PR{ID: "pr-1", Status: "approved", Agent: "nori"})
	for _, width := range []int{20, 40, 80, 120} {
		got := m.menuFooter(width)
		for i, row := range strings.Split(got, "\n") {
			if w := ansi.StringWidth(row); w != width {
				t.Errorf("width %d row %d: got %d cells, want exactly %d", width, i, w, width)
			}
		}
	}
}

// TestMenuFooterNeverSplitsAnEntry: entries pack across the two rows, but a break may only ever
// land BETWEEN entries — a hotkey separated from its label is unreadable.
func TestMenuFooterNeverSplitsAnEntry(t *testing.T) {
	m := prTabWith(api.PR{ID: "pr-1", Status: "approved", Agent: "nori"})
	offers := m.menuOffers()
	if len(offers) < 3 {
		t.Fatal("precondition: need several offers to force wrapping across rows")
	}
	wantEntries := map[string]bool{}
	for _, b := range offers {
		wantEntries[ansi.Strip(b.keys+" "+b.label(m))] = true
	}

	for _, width := range []int{15, 25, 40, 100} {
		got := ansi.Strip(m.menuFooter(width))
		got = strings.ReplaceAll(got, "\n", " · ") // the two rows read as one flowed sequence
		got = strings.TrimSuffix(strings.TrimSpace(got), "…")
		got = strings.TrimSpace(strings.TrimSuffix(got, "·"))
		for _, tok := range strings.Split(got, " · ") {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			if !wantEntries[tok] {
				t.Errorf("width %d: %q is not one whole offer — an entry was split across the wrap", width, tok)
			}
		}
	}
}

// TestMenuFooterEndsWithEllipsisWhenOffersOverflow: silent truncation would read as a complete
// menu, which here means the reader concludes an action does not apply to this row.
func TestMenuFooterEndsWithEllipsisWhenOffersOverflow(t *testing.T) {
	m := prTabWith(api.PR{ID: "pr-1", Status: "approved", Agent: "nori"})
	if len(m.menuOffers()) < 3 {
		t.Fatal("precondition: need several offers to force an overflow at a narrow width")
	}
	got := ansi.Strip(m.menuFooter(10))
	if !strings.Contains(got, "…") {
		t.Errorf("a menu narrower than its offers should end with an ellipsis, got %q", got)
	}
}

// TestMenuFooterRowsNeverGoesBlankOnAnOversizedEntry: a single entry wider than the whole footer
// used to leave both rows empty (the inner loop broke before writing anything or advancing), so
// the result read as a lone ellipsis with no clue what it hid. It must say part of the entry
// instead of dropping it silently.
func TestMenuFooterRowsNeverGoesBlankOnAnOversizedEntry(t *testing.T) {
	entries := []string{strings.Repeat("x", 50), "B short"}
	row1, row2 := menuFooterRows(entries, 10)
	if strings.TrimSpace(row1) == "" {
		t.Fatalf("row 1 should say part of the oversized entry rather than go blank, got %q / %q", row1, row2)
	}
	if w := ansi.StringWidth(row1); w != 10 {
		t.Errorf("row 1 should still be exactly the given width, got %d cells (%q)", w, row1)
	}
	if row2 != "B short" {
		t.Errorf("the second entry should still get its own row once the first advances past, got %q", row2)
	}
}
