package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/api"
)

// searchBoard is a small task tree: a parent with two children (one a search match, one not),
// a lone unrelated root, and a closed-and-stale task that would only ever surface under a wider
// filter — exactly the fixture "narrows within the filter" and "keeps ancestors visible" need.
func searchBoard() api.BoardState {
	return api.BoardState{Tasks: []api.Task{
		{ID: "sd-parent", Title: "Login epic", Status: "open"},
		{ID: "sd-child", Title: "Wire the login form", Status: "open", ParentID: "sd-parent"},
		{ID: "sd-sibling", Title: "Session handling", Status: "open", ParentID: "sd-parent"},
		{ID: "sd-other", Title: "Unrelated task", Status: "open"},
		{ID: "sd-closed", Title: "Old login bug", Status: "closed"}, // no UpdatedAt: stale, excluded by "active"
	}}
}

// searchModel is the Tasks tab, sized wide enough, on the search fixture.
func searchModel() model {
	m := newModel(nil, nil, "/r/one")
	m.tab, m.w, m.h = 0, 120, 30
	m.state = searchBoard()
	m.reclamp()
	return m
}

// press replays one keystroke through the real Update path, so modal routing (onKey vs
// updateInput) is driven exactly as live — the same technique Screenshot uses.
func press(t *testing.T, m model, k string) model {
	t.Helper()
	var tm tea.Model = m
	tm, _ = tm.Update(keyMsg(k))
	return tm.(model)
}

// typeText replays a search string one rune at a time, since real typing never arrives as one
// multi-rune message.
func typeText(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		m = press(t, m, string(r))
	}
	return m
}

// visibleIDs is the ids of every real (selectable) row currently shown.
func visibleIDs(m model) []string {
	var out []string
	for _, r := range items(m.rows()) {
		out = append(out, r.id)
	}
	return out
}

func contains(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// TestSlashOpensLiveSearchDirectly: "/" is lowercase and harmless, so it must act on the bare
// keystroke, outside the space prefix.
func TestSlashOpensLiveSearchDirectly(t *testing.T) {
	m := searchModel()
	m = press(t, m, "/")
	if m.mode != inputSearch {
		t.Fatalf("bare / should open the search field directly, mode = %v", m.mode)
	}
}

// TestSearchNarrowsAsYouType: no enter needed — every keystroke re-filters the list.
func TestSearchNarrowsAsYouType(t *testing.T) {
	m := searchModel()
	m = press(t, m, "/")
	m = typeText(t, m, "wire")

	ids := visibleIDs(m)
	if !contains(ids, "sd-child") {
		t.Fatalf("typing without pressing enter should already narrow to the match, got %v", ids)
	}
	if contains(ids, "sd-other") {
		t.Errorf("a non-matching, non-ancestor task should not appear, got %v", ids)
	}
}

// TestSearchMatchesIDOrTitleCaseInsensitively covers both fields and both cases, per the task's
// own examples ("ce09" finds sd-1ce09b; "footer" finds the space-menu task).
func TestSearchMatchesIDOrTitleCaseInsensitively(t *testing.T) {
	cases := []string{"CHILD", "child", "WIRE THE LOGIN", "wire the login"}
	for _, term := range cases {
		m := searchModel()
		m = press(t, m, "/")
		m = typeText(t, m, term)
		ids := visibleIDs(m)
		if !contains(ids, "sd-child") {
			t.Errorf("term %q should match sd-child, got %v", term, ids)
		}
	}
}

// TestSearchKeepsAncestorsVisibleAsContext: the parent doesn't match "wire" itself, but a listing
// indented by tree needs it, or the match sits at an indentation describing nothing.
func TestSearchKeepsAncestorsVisibleAsContext(t *testing.T) {
	m := searchModel()
	m = press(t, m, "/")
	m = typeText(t, m, "wire")

	ids := visibleIDs(m)
	if !contains(ids, "sd-parent") {
		t.Fatalf("the match's parent should stay visible as context, got %v", ids)
	}
	if contains(ids, "sd-sibling") {
		t.Errorf("a sibling that does not match should not be pulled in as if it were context: %v", ids)
	}
}

// TestSearchBypassesAnExistingFold: a fold armed before the search must not hide a match found
// after it — search would otherwise silently miss rows the user has no reason to remember are
// collapsed.
func TestSearchBypassesAnExistingFold(t *testing.T) {
	m := searchModel()
	m.collapsed["sd-parent"] = true
	m.reclamp()
	if contains(visibleIDs(m), "sd-child") {
		t.Fatal("precondition: the fold should hide the child before any search")
	}

	m = press(t, m, "/")
	m = typeText(t, m, "wire")
	if !contains(visibleIDs(m), "sd-child") {
		t.Errorf("a pre-existing fold must not hide a search match, got %v", visibleIDs(m))
	}
}

// TestSearchNarrowsWithinTheActiveFilter: search narrows the filtered set, it does not widen it —
// a closed, stale task matching the term still finds nothing under the default "active" filter.
func TestSearchNarrowsWithinTheActiveFilter(t *testing.T) {
	m := searchModel()
	if m.filter != api.FilterActive {
		t.Fatalf("precondition: Tasks should open on the active filter, got %q", m.filter)
	}
	m = press(t, m, "/")
	m = typeText(t, m, "login bug")
	if contains(visibleIDs(m), "sd-closed") {
		t.Errorf("a closed, stale task should stay excluded by the active filter even if it matches the search: %v", visibleIDs(m))
	}

	m.filter = api.FilterAll
	m.reclamp()
	if !contains(visibleIDs(m), "sd-closed") {
		t.Errorf("widening the filter to all should surface the same match: %v", visibleIDs(m))
	}
}

// TestSelectionPreservedByIDNotIndex: the cursor must follow the selected task through a
// re-filter, and only fall back to the first row once that task is no longer shown — getting
// this wrong means acting on the wrong task after a keystroke.
func TestSelectionPreservedByIDNotIndex(t *testing.T) {
	m := searchModel()
	m.selectRow("sd-sibling")
	if got := m.selID(); got != "sd-sibling" {
		t.Fatalf("precondition: expected sd-sibling selected, got %q", got)
	}

	m = press(t, m, "/")
	// "login" matches sd-parent (title) and sd-child, but sd-sibling is neither a match nor an
	// ancestor of one — it must drop out, and the cursor must not silently ride whatever index it
	// held to some unrelated row.
	m = typeText(t, m, "login")
	ids := visibleIDs(m)
	if contains(ids, "sd-sibling") {
		t.Fatalf("precondition: sd-sibling should have been filtered out, got %v", ids)
	}
	if got := m.selID(); got != ids[0] {
		t.Errorf("with the selected task gone, the cursor should fall back to the first row (%q), got %q", ids[0], got)
	}
}

// TestSelectionSurvivesANarrowingThatStillIncludesIt: the common case — the selected row is still
// in the narrowed set, just at a different index — must not move the cursor at all.
func TestSelectionSurvivesANarrowingThatStillIncludesIt(t *testing.T) {
	m := searchModel()
	m.selectRow("sd-other")
	m = press(t, m, "/")
	// "task" matches sd-other's title ("Unrelated task") and nothing else here.
	m = typeText(t, m, "task")
	if got := m.selID(); got != "sd-other" {
		t.Errorf("the still-visible selection should be held, got %q", got)
	}
}

// TestEscWhileTypingAbandonsAndRestores: esc during typing must throw away the in-progress term
// and put back whatever was committed before this "/" session — never a hard reset to "".
func TestEscWhileTypingAbandonsAndRestores(t *testing.T) {
	m := searchModel()
	m = press(t, m, "/")
	m = typeText(t, m, "wire")
	m = press(t, m, "enter") // commit "wire" as the standing search
	if m.mode != inputNone || m.taskSearch != "wire" {
		t.Fatalf("precondition: enter should commit and close, mode=%v search=%q", m.mode, m.taskSearch)
	}

	m = press(t, m, "/")
	m = typeText(t, m, "session") // typing something else, uncommitted
	if !contains(visibleIDs(m), "sd-sibling") {
		t.Fatalf("precondition: the live text should already be narrowing to the new term")
	}

	m = press(t, m, "esc")
	if m.mode != inputNone {
		t.Errorf("esc should close the field, mode = %v", m.mode)
	}
	if m.taskSearch != "wire" {
		t.Errorf("esc while typing should restore the previously committed term, got %q", m.taskSearch)
	}
	if !contains(visibleIDs(m), "sd-child") {
		t.Errorf("the restored view should be the previous search's, got %v", visibleIDs(m))
	}
}

// TestEnterCommitsAndKeepsTheNarrowing: enter closes the field but the list stays narrowed.
func TestEnterCommitsAndKeepsTheNarrowing(t *testing.T) {
	m := searchModel()
	m = press(t, m, "/")
	m = typeText(t, m, "wire")
	m = press(t, m, "enter")

	if m.mode != inputNone {
		t.Errorf("enter should close the field, mode = %v", m.mode)
	}
	if m.taskSearch != "wire" {
		t.Errorf("enter should commit the typed term, got %q", m.taskSearch)
	}
	if !contains(visibleIDs(m), "sd-child") {
		t.Errorf("the commit should keep the narrowing, got %v", visibleIDs(m))
	}
}

// TestEscOnACommittedSearchClearsEveryAxis: once committed, esc on the plain list clears search
// along with every other narrowing (sd-0e30bb) — a different esc from the one that merely cancels
// an open field.
func TestEscOnACommittedSearchClearsEveryAxis(t *testing.T) {
	m := searchModel()
	m = press(t, m, "/")
	m = typeText(t, m, "wire")
	m = press(t, m, "enter")
	if m.taskSearch != "wire" {
		t.Fatalf("precondition: search should be committed")
	}

	m = press(t, m, "esc")
	if m.taskSearch != "" {
		t.Errorf("esc on a plain, narrowed list should clear the search too, got %q", m.taskSearch)
	}
	if !contains(visibleIDs(m), "sd-other") {
		t.Errorf("clearing should restore the full (filtered) list, got %v", visibleIDs(m))
	}
}

// TestFilterLineNamesTheSearch: the banner must say what is narrowing the list, or an absent task
// reads as though it does not exist rather than as filtered out.
func TestFilterLineNamesTheSearch(t *testing.T) {
	m := searchModel()
	m = press(t, m, "/")
	m = typeText(t, m, "wire")
	m = press(t, m, "enter")

	line := m.filterLine()
	if !strings.Contains(line, "search: wire") {
		t.Errorf("the narrowing line should name the search term, got %q", line)
	}
}
