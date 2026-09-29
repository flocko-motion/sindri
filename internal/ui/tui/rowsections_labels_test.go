package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/table"
)

// tuiTables is every column layout the TUI lists through, by the tab that shows it. Enumerated so a
// property can be asserted of all of them at once; that each tab actually USES its own is what
// TestEveryTableIsLabelled checks, over the rows the tabs really build.
var tuiTables = map[string]table.Table{
	"tasks":  taskTable,
	"agents": agentTable,
	"prs":    prTable,
	"repos":  repoTable,
	"runs":   runsTable,
	"mail":   mailTable,
}

// TestEveryLabelFitsItsColumn: a label wider than its column would bleed into the next one, putting
// the header out of step with the very rows it names — the failure a header is supposed to prevent.
func TestEveryLabelFitsItsColumn(t *testing.T) {
	for name, tbl := range tuiTables {
		for i, c := range tbl {
			if c.Width == 0 { // the trailing column takes the rest of the line
				continue
			}
			if w := ansi.StringWidth(c.Label); w > c.Width {
				t.Errorf("%s column %d: label %q is %d cells in a column of %d", name, i, c.Label, w, c.Width)
			}
		}
	}
}

// TestOnlyTheLastColumnIsUnbounded: a zero width means "take the rest of the line", so one in the
// middle would swallow the columns after it and the header would name columns that are not there.
func TestOnlyTheLastColumnIsUnbounded(t *testing.T) {
	for name, tbl := range tuiTables {
		for i, c := range tbl {
			if c.Width == 0 && i != len(tbl)-1 {
				t.Errorf("%s column %d (%q) has no width but is not the last", name, i, c.Label)
			}
		}
	}
}

// labelledBoard gives every tab rows to label.
func labelledBoard() api.BoardState {
	b := everyTabBoard()
	b.Projects = append(b.Projects, api.Project{Tag: "oth", Path: "/r/other"})
	return b
}

// TestEveryTableIsLabelled walks the tabs that hold a table and asserts each opens with a line of
// column labels that the cursor cannot rest on. Over the rows the tabs really build, so a tab that
// lays its own out and forgets the labels fails here rather than looking fine in isolation.
func TestEveryTableIsLabelled(t *testing.T) {
	for tab, tbl := range map[int]table.Table{0: taskTable, 1: agentTable, 2: prTable, 3: repoTable, 5: runsTable, 6: mailTable} {
		m := newModel(nil, nil, "/r/ranke-db")
		m.tab, m.w, m.h = tab, 160, 30
		m.state = labelledBoard()
		m.reclamp()

		rows := m.rows()
		if len(rows) == 0 {
			t.Fatalf("%s tab: the fixture gave it no rows, so this proves nothing", tuiSections[tab].Title)
		}
		if rows[0].selectable() {
			t.Errorf("%s tab: the first line is a row, not the column labels: %q", tuiSections[tab].Title, rows[0].text)
		}
		// The labels themselves, and laid out by the table the rows use — not a lookalike built here.
		if got, want := ansi.Strip(rows[0].text), tbl.Header(); got != want {
			t.Errorf("%s tab: labels are %q, its table says %q", tuiSections[tab].Title, got, want)
		}
		for _, c := range tbl {
			if c.Label != "" && !strings.Contains(rows[0].text, c.Label) {
				t.Errorf("%s tab: label %q is missing from %q", tuiSections[tab].Title, c.Label, rows[0].text)
			}
		}
	}
}

// TestNoLabelsWithoutRows: labels over an empty table explain a table that is not there, and each
// tab's own empty state ("(no runs)") says more than a header would.
func TestNoLabelsWithoutRows(t *testing.T) {
	for tab := range tuiSections {
		m := newModel(nil, nil, "/r/ranke-db")
		m.tab, m.w, m.h = tab, 160, 30
		m.state = api.BoardState{} // nothing registered and nothing on the board
		m.reclamp()
		if rows := m.rows(); len(rows) != 0 {
			t.Errorf("%s tab: an empty board should give no rows, got %d: %q",
				tuiSections[tab].Title, len(rows), rowTexts(rows))
		}
	}
}

// TestMailReadsFromThenTo: the two columns are adjacent, and in the other order they were misread
// repeatedly. Labelling a backwards order would only have made the backwardness legible.
func TestMailReadsFromThenTo(t *testing.T) {
	from, to := -1, -1
	for i, c := range mailTable {
		switch c.Label {
		case "from":
			from = i
		case "to":
			to = i
		}
	}
	if from < 0 || to < 0 {
		t.Fatalf("the mail list must label its sender and recipient, got %v", mailTable)
	}
	if from > to {
		t.Error("sender belongs before recipient, the order mail is read in everywhere else")
	}
}

// TestTheCursorCannotLandOnTheLabels, on every tab that has them, by every key that moves it. The
// same constraint the section headings carry, through the same mechanism — a list has one kind of
// line that is not a row.
func TestTheCursorCannotLandOnTheLabels(t *testing.T) {
	for _, tab := range []int{0, 1, 2, 3, 5, 6} {
		m := newModel(nil, nil, "/r/ranke-db")
		m.tab, m.w, m.h = tab, 160, 30
		m.state = labelledBoard()
		m.reclamp()

		for _, key := range []string{"g", "k", "j", "j", "j", "G", "ctrl+u", "ctrl+d", "g"} {
			m.onKey(key)
			m.reclamp()
			rows := m.rows()
			if c := m.cursor[tab]; c < 0 || c >= len(rows) || !rows[c].selectable() {
				t.Errorf("%s tab: %q left the cursor at %d of %d rows, on a line that selects nothing",
					tuiSections[tab].Title, key, c, len(rows))
			}
			if m.selID() == "" {
				t.Errorf("%s tab: %q left nothing selected", tuiSections[tab].Title, key)
			}
		}
	}
}

// TestThePRsTabTakesTheSharedColumnSet: the guard above is about THIS package's tables, and the drift
// it missed was between packages — a "for" column on this tab and not on `sindri pr list`. Pinning
// that prTable is the shared set catches a hand-rolled table growing back here (-> table.PRList).
func TestThePRsTabTakesTheSharedColumnSet(t *testing.T) {
	shared := table.PRList(9)
	if len(prTable) != len(shared) {
		t.Fatalf("the PRs tab has %d columns, the shared set has %d", len(prTable), len(shared))
	}
	for i, want := range shared {
		if prTable[i] != want {
			t.Errorf("column %d is %+v, want the shared %+v", i, prTable[i], want)
		}
	}
}
