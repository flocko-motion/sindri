package tui

import (
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/flo-at/sindri/internal/api"
)

// tasksTabWith puts one task on the Tasks tab, selected.
func tasksTabWith(tasks ...api.Task) model {
	m := newModel(nil, nil, "/r/one")
	m.tab, m.filter = 0, api.FilterAll
	m.state = api.BoardState{Tasks: tasks}
	m.reclamp()
	return m
}

// TestACommittingKeyIsInertOnItsOwn is what the prefix buys: the keystroke that used to change
// something now does nothing until the menu is open. Two deliberate presses, and the menu names
// what became possible in between.
func TestACommittingKeyIsInertOnItsOwn(t *testing.T) {
	m := tasksTabWith(api.Task{ID: "td-1", Title: "a task", Status: "open", Approval: "pending"})
	m.onKey(keyApprove) // bare A: the old gesture, which used to approve
	if m.choice.active || m.form.active || m.flash != "" {
		t.Errorf("a bare committing key must do nothing: choice=%v form=%v flash=%q",
			m.choice.active, m.form.active, m.flash)
	}

	m.onKey(keyMenu)
	if !m.menu {
		t.Fatal("space should open the menu")
	}
	m.onKey(keyApprove)
	if m.menu {
		t.Error("acting closes the menu")
	}
	if !m.choice.active && m.flash == "" {
		t.Error("space then A should reach approve, which the bare key no longer does")
	}
}

// TestEscapeCancelsTheMenu: opening it commits to nothing, so leaving costs nothing either.
func TestEscapeCancelsTheMenu(t *testing.T) {
	for _, cancel := range []string{"esc", keyMenu} {
		m := tasksTabWith(api.Task{ID: "td-1", Status: "open", Approval: "pending"})
		m.onKey(keyMenu)
		m.onKey(cancel)
		if m.menu {
			t.Errorf("%q should close the menu", cancel)
		}
		if m.choice.active || m.form.active || m.flash != "" {
			t.Errorf("%q should change nothing on the way out", cancel)
		}
	}
}

// TestNavigationIsUntouchedByThePrefix: lowercase still works directly, and it works while the menu
// is open only in the sense that it closes it — the menu is a modal answer to "what can I commit",
// so a navigation key is not silently swallowed into a mutation.
func TestNavigationIsUntouchedByThePrefix(t *testing.T) {
	m := tasksTabWith(api.Task{ID: "td-1", Status: "open"})
	m.onKey(keyFilter) // f: cycles the view, no prefix needed
	if m.filter == api.FilterAll {
		t.Error("f should still cycle the filter directly")
	}
	m.onKey(keyMenu)
	m.onKey(keyFilter) // a letter the menu never offered
	if m.menu {
		t.Error("a key the menu does not offer still closes it")
	}
}

// TestTheMenuOffersOnlyWhatAppliesToTheRow is what makes it better than a flat list: it answers
// "what can I do with this" rather than "what exists". An action the hub would refuse is invisible.
func TestTheMenuOffersOnlyWhatAppliesToTheRow(t *testing.T) {
	// Nobody holds this task, so there is nothing to unassign; it is open, so it can be closed.
	m := tasksTabWith(api.Task{ID: "td-1", Title: "unheld", Status: "open"})
	if menuHas(m, keyUnassign+" unassign") {
		t.Errorf("nobody holds td-1, so unassign should not be offered:\n%s", menuText(m))
	}
	if !menuHas(m, keyClose+" close") {
		t.Errorf("an open task can be closed:\n%s", menuText(m))
	}

	// Closed: the reverse pair — reopen appears, close does not.
	m = tasksTabWith(api.Task{ID: "td-1", Title: "done", Status: "closed"})
	if menuHas(m, keyClose+" close") {
		t.Errorf("a closed task cannot be closed again:\n%s", menuText(m))
	}
	// A task nobody has proposed has no verdict to give.
	if menuHas(m, keyApprove+" approve") {
		t.Errorf("no approval is pending on td-1:\n%s", menuText(m))
	}
	pending := tasksTabWith(api.Task{ID: "td-2", Title: "proposed", Status: "open", Approval: "pending"})
	if !menuHas(pending, keyApprove+" approve") {
		t.Errorf("a task awaiting a verdict offers one:\n%s", menuText(pending))
	}
}

// TestTheFooterCarriesNavigationAndOneEntry: the footer had run out of room — eight working
// bindings were never advertised at all — and collapsing the committing ones behind one entry,
// now named on the GLOBAL row since the prefix works on every tab, is what buys the space back.
// A tab-local row carries only its own navigation, never the prefix or a committing binding.
func TestTheFooterCarriesNavigationAndOneEntry(t *testing.T) {
	for _, tc := range []struct {
		tab   int
		scope keyScope
	}{
		{0, scopeTasks}, {1, scopeAgents}, {2, scopePRs}, {3, scopeRepos}, {4, scopeChat}, {5, scopeRuns},
	} {
		m := newModel(nil, nil, "/r/one")
		m.tab = tc.tab
		global := m.globalFooter(500)
		if !strings.Contains(global, keyMenuShown+" actions") {
			t.Errorf("tab %d's global footer should name the prefix readably:\n%s", tc.tab, global)
		}
		local := footerOf(t, tc.scope)
		if strings.Contains(local, keyMenuShown) {
			t.Errorf("scope %d's local footer should no longer carry the prefix — it is global now:\n%s", tc.scope, local)
		}
		for _, b := range keymap {
			if b.scope != tc.scope || !b.commits {
				continue
			}
			if strings.Contains(local, b.keys+" "+b.label(m)) {
				t.Errorf("scope %d still advertises the committing %q in its footer:\n%s", tc.scope, b.keys, local)
			}
		}
	}
}

// TestGlobalFooterShedsWholeEntriesWhenNarrow: the row must stay exactly the terminal width, so a
// narrow one drops whole entries — never truncates one mid-word — and says so with a trailing "…"
// between the pinned lead and trail.
func TestGlobalFooterShedsWholeEntriesWhenNarrow(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	full := m.globalFooter(500)
	wantEntries := map[string]bool{"…": true}
	for _, e := range strings.Split(full, " · ") {
		wantEntries[e] = true
	}
	if len(wantEntries) < 4 {
		t.Fatal("precondition: need several global entries to force shedding")
	}
	for _, width := range []int{30, 40, 60} {
		got := m.globalFooter(width)
		if w := ansi.StringWidth(got); w > width {
			t.Errorf("width %d: global footer is %d cells wide: %q", width, w, got)
		}
		for _, tok := range strings.Split(got, " · ") {
			if tok != "" && !wantEntries[tok] {
				t.Errorf("width %d: %q is not one whole entry — the row split one:\n%s", width, tok, got)
			}
		}
	}
}

// TestGlobalFooterKeepsHelpAndPrefixAtOrdinaryWidths: shedding from the tail would drop the prefix
// first, since it is appended last — invisible below 180 columns on the Tasks tab, where it used
// to ride the much shorter tab-local row. Both ends are pinned, so an ordinary terminal (80, 100,
// 120 columns) must still show both "? help" and the prefix entry.
func TestGlobalFooterKeepsHelpAndPrefixAtOrdinaryWidths(t *testing.T) {
	for _, tab := range []int{0, 1, 2, 3, 4, 5, 6} {
		m := newModel(nil, nil, "/r/one")
		m.tab = tab
		for _, width := range []int{80, 100, 120} {
			got := m.globalFooter(width)
			if w := ansi.StringWidth(got); w > width {
				t.Fatalf("tab %d width %d: global footer is %d cells wide: %q", tab, width, w, got)
			}
			if !strings.HasPrefix(got, keyHelp+" help") {
				t.Errorf("tab %d width %d: ? help should lead the row, got %q", tab, width, got)
			}
			if !strings.Contains(got, keyMenuShown+" actions") {
				t.Errorf("tab %d width %d: the prefix entry should still be named, got %q", tab, width, got)
			}
		}
	}
}

// TestGlobalFooterShedsByUsefulnessNotDeclarationOrder is sd-5e3032's own instruction: "shed the
// least useful entries". Shedding by declaration order instead dropped movement (j/k/g/G) at 80
// columns — the default terminal width — while keeping tab-jump and pane-switching, which the task
// singles out as the least useful. globalShedOrder must protect basic movement and quit before
// anything else goes.
func TestGlobalFooterShedsByUsefulnessNotDeclarationOrder(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab = 0
	got := m.globalFooter(80)
	for _, want := range []string{"move/top/bot", keyQuit + " quit"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q should survive at 80 columns, got %q", want, got)
		}
	}
	for _, gone := range []string{"1-7 jump", "pane"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q was named the least useful and should be first to go at 80 columns, got %q", gone, got)
		}
	}
}

// TestGlobalFooterShedsByUsefulnessUnderFocusToo: focusSplit mints display labels
// ("item", "goto", "copy", "bottom") globalShedOrder has never heard of. Ranking by that
// display label instead of the underlying binding's own label let those parts outrank named
// entries by accident — at 80 columns focused, movement (now "G bottom", ranked through its
// parent's ordinary "move/top/bot" label) was shed while "y copy" (unranked, so sorted last)
// survived. globalEntry.rank fixes this by tracking a part back to its parent binding's label
// for shedPriority, regardless of which override text that part actually displays; this pins it.
func TestGlobalFooterShedsByUsefulnessUnderFocusToo(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	m.tab = 0
	m.state = api.BoardState{Tasks: []api.Task{{ID: "td-1", Status: "open"}}}
	m.reclamp()
	m.focus = focusItems
	got := m.globalFooter(80)
	for _, want := range []string{"G bottom", keyQuit + " quit"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q should survive at 80 columns while focused, got %q", want, got)
		}
	}
	for _, gone := range []string{"1-7 jump", "pane"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q should still be shed first at 80 columns while focused, got %q", gone, got)
		}
	}
}

// TestHelpKeyLeadsTheGlobalRow: "?" is the first thing a reader sees, and so the first thing kept
// once the row has to shed the rest.
func TestHelpKeyLeadsTheGlobalRow(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	full := m.globalFooter(500)
	if !strings.HasPrefix(full, keyHelp+" help") {
		t.Errorf("? help should lead the global row, got %q", full)
	}
	narrow := m.globalFooter(20)
	if !strings.HasPrefix(narrow, keyHelp+" help") {
		t.Errorf("? help should survive shedding at a realistic narrow width, got %q", narrow)
	}
}

// TestNothingLowercaseCommits is the invariant the reworded convention states: a key that navigates
// is direct and a key that commits is behind the prefix, so no lowercase binding may be a
// committing one. The allowlist in TestLowercaseKeysNeverMutate stays as the fail-closed half —
// this is the structural half, and it needs no list to maintain.
func TestNothingLowercaseCommits(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	for _, b := range keymap {
		if !b.commits {
			continue
		}
		for _, k := range menuKeys(b.keys) {
			r := []rune(k)[0]
			if unicode.IsLetter(r) && unicode.IsLower(r) {
				t.Errorf("%q commits but is lowercase (label %q) — a committing key belongs behind the prefix",
					k, b.label(m))
			}
		}
	}
}

// TestOneActionIsClassifiedOnce is the guard the E slip walked past: whether a key commits may
// differ BETWEEN tabs, because the action does — A approves a PR and opens a member picker on the
// Meeting tab — but the same key with the same label is the same action, and it cannot navigate on
// one tab and commit on another. E "config" opened the same prefilled form on both, and was direct
// globally while inert-until-the-menu on Repos: one key, one implementation, two answers.
func TestOneActionIsClassifiedOnce(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	seen := map[string]struct {
		commits bool
		scope   keyScope
	}{}
	for _, b := range keymap {
		id := b.keys + "\x00" + b.label(m)
		first, ok := seen[id]
		if !ok {
			seen[id] = struct {
				commits bool
				scope   keyScope
			}{b.commits, b.scope}
			continue
		}
		if first.commits != b.commits {
			t.Errorf("%q (%s) commits=%v in scope %d but commits=%v in scope %d — same key and same "+
				"label is the same action, so it cannot be both",
				b.keys, b.label(m), first.commits, first.scope, b.commits, b.scope)
		}
	}
}

// TestEveryWhenGatedBindingHasWhenText holds tasks.md's invariant: a binding narrowed by `when`
// must spell the condition out for the help modal, or a future one ships silently condition-less.
func TestEveryWhenGatedBindingHasWhenText(t *testing.T) {
	m := newModel(nil, nil, "/r/one")
	for _, b := range keymap {
		if b.when != nil && b.whenText == "" {
			t.Errorf("%q (%s) has a when-clause but no whenText for the help modal", b.keys, b.label(m))
		}
	}
}
