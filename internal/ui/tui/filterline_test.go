package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// narrowedModel is a tab of the two-repo board, at its default filters.
func narrowedModel(tab int) model {
	m := newModel(nil, nil, "/r/here")
	m.tab, m.w, m.h = tab, 120, 40
	m.state = mailJumpBoard()
	m.reclamp()
	return m
}

// TestTheOrdinaryViewSaysNothing is the constraint that keeps the line worth reading: a banner on
// every list would spend a row explaining the usual case, and an eye stops reading a line that is
// always there.
func TestTheOrdinaryViewSaysNothing(t *testing.T) {
	for _, tab := range []int{0, 1, 2, 3, 5, 6} {
		if line := narrowedModel(tab).filterLine(); line != "" {
			t.Errorf("tab %d at its defaults shows %q", tab, line)
		}
	}
}

// TestANarrowedViewNamesEveryAxis: the axis that raised the line is rarely the only one hiding
// rows, so a line naming one of two would send the reader after the wrong cause.
func TestANarrowedViewNamesEveryAxis(t *testing.T) {
	m := narrowedModel(6)
	m.mailAgent = "dvalin" // the only non-default axis
	line := m.filterLine()
	for _, want := range []string{"filter: " + string(api.MailFilters[0]), "to dvalin", "repo here"} {
		if !strings.Contains(line, want) {
			t.Errorf("the line should name %q, got %q", want, line)
		}
	}
	if !strings.Contains(line, keyClearFilters) {
		t.Errorf("the way out belongs on the line, got %q", line)
	}
}

// TestTheLineSitsAboveTheRowsAndSelectsNothing: it is the third user of the unselectable-line
// mechanism, so the cursor must walk past it exactly as it does a column label.
func TestTheLineSitsAboveTheRowsAndSelectsNothing(t *testing.T) {
	m := narrowedModel(6)
	m.mailAgent = "dwalin"
	m.scopeRepo = false
	rows := m.rows()
	if len(rows) == 0 {
		t.Fatal("precondition: the narrowed view still has rows")
	}
	if rows[0].selectable() {
		t.Error("the filter line must select nothing, or the cursor can rest on it")
	}
	if !strings.Contains(rows[0].text, "to dwalin") {
		t.Errorf("the line should come first, above the column labels: %q", rows[0].text)
	}
	if id := m.selID(); id == "" {
		t.Error("the cursor should have snapped past the line onto a real row")
	}
}

// TestAnEmptyNarrowedListStillExplainsItself is the worst case of the confusion this fixes: zero
// rows and no statement reads as "there is nothing", when the truth is "you filtered it all out".
func TestAnEmptyNarrowedListStillExplainsItself(t *testing.T) {
	m := narrowedModel(6)
	m.mailAgent = "nobody-by-that-name"
	rows := m.rows()
	if len(rows) != 1 {
		t.Fatalf("an emptied list should hold only its filter line, got %d rows", len(rows))
	}
	if !strings.Contains(rows[0].text, "to nobody-by-that-name") {
		t.Errorf("it should say what emptied the list: %q", rows[0].text)
	}
}

// TestEscClearsEveryAxisAtOnce: one key, or the user clears axes one at a time by finding the key
// for each — which is the state this task exists to end.
func TestEscClearsEveryAxisAtOnce(t *testing.T) {
	m := narrowedModel(6)
	m.mailAgent, m.mailFilter, m.scopeRepo = "dwalin", api.MailAll, false
	m.filter, m.prFilter, m.runFilter = api.FilterClosed, api.PRFilterAll, api.RunFilterAll

	m.onKey(keyClearFilters)

	if line := m.filterLine(); line != "" {
		t.Errorf("esc should leave the ordinary view, still narrowed by %q", line)
	}
	for name, got := range map[string]bool{
		"mailAgent":  m.mailAgent == "",
		"mailFilter": m.mailFilter == api.MailFilters[0],
		"scopeRepo":  m.scopeRepo,
		"filter":     m.filter == api.FilterActive,
		"prFilter":   m.prFilter == api.PRFilterActive,
		"runFilter":  m.runFilter == api.RunFilterActive,
	} {
		if !got {
			t.Errorf("%s was not put back to what the tab opens with", name)
		}
	}
}

// TestClearingRestoresTheDEFAULTNotTheWidest: widening to "all" would leave a filter the user then
// has to clear in turn, so the line would still be there and the key's promise unkept.
func TestClearingRestoresTheDEFAULTNotTheWidest(t *testing.T) {
	m := narrowedModel(0)
	m.filter = api.FilterAll
	if m.filterLine() == "" {
		t.Fatal("precondition: 'all' is not the default, so it raises the line")
	}
	m.onKey(keyClearFilters)
	if m.filter != api.FilterActive {
		t.Errorf("filter = %q, want the tab's default", m.filter)
	}
}

// TestEscOnAnOrdinaryViewChangesNothing: it is bound on every list, so on an unnarrowed one it must
// not quietly reset a scope the user set on another tab.
func TestEscOnAnOrdinaryViewChangesNothing(t *testing.T) {
	m := narrowedModel(0) // Tasks: its own filter is default, and scope is not one of its axes
	m.scopeRepo = false   // set for the Agents/PRs tabs, and nothing to do with this list
	m.onKey(keyClearFilters)
	if m.scopeRepo {
		t.Error("esc on a view that is not narrowed should do nothing at all")
	}
}

// TestEscStillCancelsTheMenuPrefix: esc had one meaning on a list already — backing out of the
// space menu — and taking it for the filters must not cost that.
func TestEscStillCancelsTheMenuPrefix(t *testing.T) {
	m := narrowedModel(6)
	m.mailAgent = "dwalin" // narrowed, so the new binding would fire if it were reached
	m.onKey(keyMenu)
	if !m.menu {
		t.Fatal("precondition: space opens the menu")
	}
	m.onKey(keyClearFilters)
	if m.menu {
		t.Error("esc should have closed the menu")
	}
	if m.mailAgent != "dwalin" {
		t.Error("cancelling the menu must not also clear the filters — one keypress, one meaning")
	}
}

// TestEscSaysHowFarItReaches: it clears every tab, but the line above the rows names only this
// tab's axes, so a user clearing a mail narrowing loses a PR filter the line never mentioned. In a
// change whose whole theme is that a narrowing must be honest about itself, the key that undoes one
// has to be honest about its reach.
func TestEscSaysHowFarItReaches(t *testing.T) {
	m := narrowedModel(6)
	m.mailAgent, m.prFilter = "dwalin", api.PRFilterAll
	m.onKey(keyClearFilters)
	if !strings.Contains(m.flash, "every tab") {
		t.Errorf("esc reached past this tab's axes without saying so, got %q", m.flash)
	}
}

// TestTheMailDefaultIsReadWhereItIsDefined: the value the tab opens with is written in api, and a
// copy of it here would go stale the day it changes — at which point the line would start calling
// the default a narrowing, and esc would "clear" to something the view never opens on.
func TestTheMailDefaultIsReadWhereItIsDefined(t *testing.T) {
	m := narrowedModel(6)
	if m.mailFilter != api.MailFilters[0] {
		t.Fatalf("the Mail tab opens on %q, which is not what api says a view opens on", m.mailFilter)
	}
	m.mailFilter = api.NextMailFilter(api.MailFilters[0])
	if m.filterLine() == "" {
		t.Error("a filter that is not the default should raise the line")
	}
	m.onKey(keyClearFilters)
	if m.mailFilter != api.MailFilters[0] {
		t.Errorf("clearing put the filter back to %q rather than what the view opens on", m.mailFilter)
	}
}
