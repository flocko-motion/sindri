package tui

import (
	"strconv"
	"strings"
	"testing"
)

// TestDigitJumpsToTab is derived over tuiSections rather than naming seven cases by hand: digit N
// (1-indexed) must land on tab N-1, for every tab that exists — so an eighth tab added later is
// covered by the same test rather than needing a new case.
func TestDigitJumpsToTab(t *testing.T) {
	for i := range tuiSections {
		m := newModel(nil, nil, "/r/one")
		m.tab = (i + 3) % len(tuiSections) // start somewhere else, so landing on i proves the jump
		m.onKey(strconv.Itoa(i + 1))
		if m.tab != i {
			t.Errorf("digit %d should land on tab %d (%s), got %d", i+1, i, tuiSections[i].Title, m.tab)
		}
	}
}

// TestOutOfRangeDigitsLeaveTheTabAlone: 8, 9 and 0 have nothing to jump to (with 7 tabs) and must
// not clamp to the last one — a clamp turns a mistyped key into a silent jump the user never asked
// for.
func TestOutOfRangeDigitsLeaveTheTabAlone(t *testing.T) {
	for _, k := range []string{"8", "9", "0"} {
		m := newModel(nil, nil, "/r/one")
		m.tab = 2
		m.onKey(k)
		if m.tab != 2 {
			t.Errorf("%q should leave the tab alone (7 tabs, nothing to jump to), got tab %d", k, m.tab)
		}
	}
}

// TestDigitsAreDirectNotBehindThePrefix: harmless navigation stays outside the space menu, so a
// bare digit must act at once.
func TestDigitsAreDirectNotBehindThePrefix(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab = 0
	m.onKey("3")
	if m.tab != 2 {
		t.Fatalf("a bare digit should jump directly, got tab %d", m.tab)
	}
}

// TestTabLabelsLeadWithTheHotkeyNumber: the header must advertise the same number onKey answers
// to, in the same order tuiSections lists them — the order the header already shows.
func TestTabLabelsLeadWithTheHotkeyNumber(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	labels := m.tabLabels()
	if len(labels) != len(tuiSections) {
		t.Fatalf("got %d labels, want one per section (%d)", len(labels), len(tuiSections))
	}
	for i, s := range tuiSections {
		want := strconv.Itoa(i + 1)
		if !strings.HasPrefix(labels[i], want+" ") {
			t.Errorf("label %d (%s) = %q, want it to lead with %q", i, s.Title, labels[i], want)
		}
		if !strings.Contains(labels[i], s.Title) {
			t.Errorf("label %d = %q, missing its title %q", i, labels[i], s.Title)
		}
		// A second bare digit right after the hotkey (the count badge, "1 12 Tasks") is
		// indistinguishable from a second hotkey digit — the title must separate the two.
		if rest := strings.TrimPrefix(labels[i], want+" "); rest != "" && rest[0] >= '0' && rest[0] <= '9' {
			t.Errorf("label %d = %q: a bare number sits right after the hotkey with nothing between them", i, labels[i])
		}
	}
}

// TestJumpIsAdvertisedGlobally: the binding must be declared in the keymap, not only in the
// dispatcher, or the footer and the actual behaviour can drift apart. It is global — shown on
// every tab, not one scope's row — and its range must track tuiSections rather than a hand count.
func TestJumpIsAdvertisedGlobally(t *testing.T) {
	footer := footerOf(t, scopeGlobal)
	want := "1-" + strconv.Itoa(len(tuiSections)) + " jump"
	if !strings.Contains(footer, want) {
		t.Errorf("the global footer should advertise %q:\n%s", want, footer)
	}
}
