package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/api"
	"github.com/muesli/termenv"
)

// itemRows counts the rows the cursor can rest on. A badge counts items, and a grouped list also
// carries the headings and spacers that label them.
func itemRows(rows []row) int {
	n := 0
	for _, r := range rows {
		if r.selectable() {
			n++
		}
	}
	return n
}

// headingIndexes is where the unselectable rows are, so a test can say what the cursor must not
// land on without knowing how a heading is styled.
func headingIndexes(rows []row) []int {
	var out []int
	for i, r := range rows {
		if !r.selectable() {
			out = append(out, i)
		}
	}
	return out
}

// foreignBoard: the selected repo holds a working agent and an open PR; another repo holds one of
// each that waits on the user, plus one of each that does not. The shape the complaint came from —
// rows from elsewhere sitting in a repo-scoped list with nothing but a repo tag to place them.
func foreignBoard() (model, api.BoardState) {
	m := newModel(nil, nil, "/r/sindri")
	m.scopeRepo = true
	b := api.BoardState{
		Projects: []api.Project{
			{Tag: "sin", Path: "/r/sindri"},
			{Tag: "oth", Path: "/r/other"},
		},
		Agents: []api.AgentView{
			{Name: "eitri", Project: "sin", Repo: "sindri", Status: "working"},
			{Name: "gloin", Project: "oth", Repo: "other", Status: "working"},
			{Name: "thrain", Project: "oth", Repo: "other", Status: api.StatusFull},
			{Name: "nori", Project: "oth", Repo: "other", Status: api.StatusBlocked},
			// A reviewer up in "oth", so the foreign open PR there waits on nobody.
			{Name: "regin", Project: "oth", Repo: "other", Role: "reviewer", Status: "working"},
		},
		PRs: []api.PR{
			{ID: "pr-1", Project: "sin", Status: "open"},
			{ID: "pr-2", Project: "oth", Status: "open"},     // a reviewer runs there: not the user's
			{ID: "pr-3", Project: "oth", Status: "approved"}, // only the user can merge it
		},
	}
	return m, b
}

// TestForeignRowsSitUnderALabelledHeading: the rows were already grouped and still misread, because
// nothing said what the group was. The heading is the whole fix, so it is what is pinned — above the
// foreign rows, with a labelled local section beneath them.
func TestForeignRowsSitUnderALabelledHeading(t *testing.T) {
	m, b := foreignBoard()
	m.state = b

	for _, c := range []struct {
		tab  string
		rows []row
	}{{"agents", m.agentRows()}, {"prs", m.prRows()}} {
		texts := rowTexts(c.rows)
		joined := strings.Join(texts, "\n")
		foreign, local := -1, -1
		for i, text := range texts {
			if strings.Contains(text, "Needing attention in other repos") {
				foreign = i
			}
			if strings.Contains(text, api.LocalHeading) {
				local = i
			}
		}
		if foreign < 0 {
			t.Fatalf("%s: no heading over the foreign rows:\n%s", c.tab, joined)
		}
		if local < 0 {
			t.Fatalf("%s: the local rows are unlabelled:\n%s", c.tab, joined)
		}
		if foreign > local {
			t.Errorf("%s: the foreign rows must come first — they are on screen because they need the user:\n%s", c.tab, joined)
		}
		if strings.TrimSpace(texts[local-1]) != "" {
			t.Errorf("%s: the sections need a blank line between them, got %q above %q", c.tab, texts[local-1], texts[local])
		}
	}
}

// TestTheForeignHeadingCarriesItsCount: the "(N!)" badge on the handle says how many wait on the
// user and the heading says which they are. One claim, two renderings — so the heading counts the
// rows beneath it, and those rows plus the local ones needing the user are what the badge counts.
func TestTheForeignHeadingCarriesItsCount(t *testing.T) {
	m, b := foreignBoard()
	m.state = b

	for _, c := range []struct {
		tab       string
		rows      []row
		attention int
	}{
		{"agents", m.agentRows(), api.CountAgentsNeedingUser(b.Agents)},
		{"prs", m.prRows(), api.CountPRsNeedingUser(b.PRs, b.Agents)},
	} {
		texts := rowTexts(c.rows)
		var foreign int
		for i, text := range texts {
			if strings.Contains(text, "Needing attention in other repos") {
				foreign = countRowsUntilBlank(texts[i+1:])
				if !strings.Contains(text, api.ForeignAttentionHeading(foreign)) {
					t.Errorf("%s: heading %q does not count the %d rows under it", c.tab, text, foreign)
				}
			}
		}
		if foreign == 0 {
			t.Fatalf("%s: the fixture has foreign rows waiting on the user; none were grouped", c.tab)
		}
		if foreign > c.attention {
			t.Errorf("%s: the heading counts %d rows but only %d wait on the user fleet-wide", c.tab, foreign, c.attention)
		}
	}
}

// countRowsUntilBlank is how many rows a section holds: everything up to the spacer that ends it.
func countRowsUntilBlank(texts []string) int {
	for i, text := range texts {
		if strings.TrimSpace(text) == "" {
			return i
		}
	}
	return len(texts)
}

// TestNoHeadingsWhenEverythingIsLocal: the ordinary glance is the one to protect. With nothing
// waiting elsewhere the lists must read exactly as they did — no heading, no spacer.
func TestNoHeadingsWhenEverythingIsLocal(t *testing.T) {
	m, b := foreignBoard()
	b.Agents = []api.AgentView{{Name: "eitri", Project: "sin", Repo: "sindri", Status: api.StatusFull}}
	b.PRs = []api.PR{{ID: "pr-1", Project: "sin", Status: "approved"}}
	m.state = b

	// Stuck locally, so the badge is marked and the exception still has nothing to admit.
	for _, c := range []struct {
		tab  string
		rows []row
	}{{"agents", m.agentRows()}, {"prs", m.prRows()}} {
		if n := len(headingIndexes(c.rows)); n != 0 {
			t.Errorf("%s: %d unselectable row(s) in a purely local list:\n%s", c.tab, n, strings.Join(rowTexts(c.rows), "\n"))
		}
	}
}

// TestGlobalScopeHasNoForeignSection: global scope admits every row on its own merit, so no row is
// there only because it needs the user and there is nothing for a heading to explain.
func TestGlobalScopeHasNoForeignSection(t *testing.T) {
	m, b := foreignBoard()
	m.state = b
	m.scopeRepo = false

	for _, c := range []struct {
		tab  string
		rows []row
	}{{"agents", m.agentRows()}, {"prs", m.prRows()}} {
		if n := len(headingIndexes(c.rows)); n != 0 {
			t.Errorf("%s: global scope should group nothing, got %d unselectable row(s)", c.tab, n)
		}
	}
}

// TestTheCursorCannotLandOnAHeading is the gotcha. The cursor walks the row list, so a heading it
// could rest on is a selection with no item behind it and a detail pane with nothing to show.
func TestTheCursorCannotLandOnAHeading(t *testing.T) {
	m, b := foreignBoard()
	m.state = b
	m.w, m.h = 200, 40

	for _, tab := range []int{1, 2} {
		m.tab = tab
		if len(headingIndexes(m.rows())) == 0 {
			t.Fatalf("tab %d: the fixture must produce headings for this to test anything", tab)
		}
		// Every key that moves the selection, from either end of the list.
		for _, key := range []string{"g", "j", "j", "j", "j", "j", "j", "G", "k", "k", "k", "k", "ctrl+u", "ctrl+d"} {
			m.onKey(key)
			m.reclamp()
			rows := m.rows()
			c := m.cursor[m.tab]
			if c < 0 || c >= len(rows) {
				t.Fatalf("tab %d: %q put the cursor at %d, outside %d rows", tab, key, c, len(rows))
			}
			if !rows[c].selectable() {
				t.Errorf("tab %d: %q landed the cursor on %q, which selects nothing", tab, key, rows[c].text)
			}
			if m.selID() == "" {
				t.Errorf("tab %d: %q left nothing selected, so the detail pane has nothing to show", tab, key)
			}
		}
	}
}

// TestTheFirstFrameOpensOnAnItem: a fresh model starts at cursor 0, which in a grouped list is the
// foreign heading. Nothing has pressed a key yet, so the snap has to happen on layout too.
func TestTheFirstFrameOpensOnAnItem(t *testing.T) {
	m, b := foreignBoard()
	m.state = b
	m.w, m.h = 200, 40

	for _, tab := range []int{1, 2} {
		m.tab = tab
		m.cursor[tab] = 0
		m.reclamp()
		if id := m.selID(); id == "" {
			t.Errorf("tab %d: the first frame selects nothing — cursor rests at %d", tab, m.cursor[tab])
		}
	}
}

// foreignScreen renders foreignBoard through the real update path, in a repo scope that has rows
// from elsewhere. Screenshot() cannot: it starts the model outside any repo, where the scope has no
// repo to narrow to and nothing is foreign.
func foreignScreen(w, h int, keys ...string) string {
	m, b := foreignBoard()
	m.w, m.h = w, h
	m.state = b
	m.reclamp()
	var tm tea.Model = m
	for _, k := range keys {
		tm, _ = tm.Update(keyMsg(k))
	}
	return tm.View()
}

// TestForeignSectionScreenshot prints both grouped lists so the labelling can be eyeballed:
//
//	go test ./internal/ui/tui/ -run ForeignSectionScreenshot -v
func TestForeignSectionScreenshot(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii) // plain text, no escape codes
	for _, sc := range []struct {
		name string
		keys []string
	}{
		{"Agents tab — repo scope, two agents stuck elsewhere", []string{"tab"}},
		{"PRs tab — repo scope, a PR waiting on you elsewhere", []string{"tab", "tab"}},
	} {
		fmt.Printf("\n========== %s ==========\n%s\n", sc.name, foreignScreen(120, 20, sc.keys...))
	}
}
