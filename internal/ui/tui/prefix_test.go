package tui

import (
	"strings"
	"testing"
	"unicode"

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
// bindings were never advertised at all — and collapsing the committing ones behind one entry is
// what buys the space back.
func TestTheFooterCarriesNavigationAndOneEntry(t *testing.T) {
	for _, scope := range []keyScope{scopeTasks, scopeAgents, scopePRs, scopeRepos, scopeChat, scopeRuns} {
		footer := footerOf(t, scope)
		if !strings.Contains(footer, keyMenuShown+" actions") {
			t.Errorf("scope %d's footer should name the prefix readably:\n%s", scope, footer)
		}
		for _, b := range keymap {
			if b.scope != scope || !b.commits {
				continue
			}
			if strings.Contains(footer, b.keys+" "+b.label(newModel(nil, nil, "/r/one"))) {
				t.Errorf("scope %d still advertises the committing %q in its footer:\n%s", scope, b.keys, footer)
			}
		}
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
