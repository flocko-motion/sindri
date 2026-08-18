// package: tui / component_menu
// type:    ui component (the space-prefix action menu)
// job:     show the committing actions available for what is selected, and make their
// letters live while it is open — the prefix that keeps every state change two
// deliberate presses away.
// limits:  chrome and the offer list; the actions themselves stay in onKey, and which
// bindings commit is declared in the keymap (-> keys.go).
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/api"
)

// menuKeys splits a binding's displayed keys into the ones a press can match: "A/R" offers both A
// and R, and a row like "C-h/C-l" offers neither — those are prose for the footer, not letters.
func menuKeys(display string) []string {
	var out []string
	for _, part := range strings.Split(display, "/") {
		if len([]rune(part)) == 1 {
			out = append(out, part)
		}
	}
	return out
}

// menuOffers is what the space menu shows for the current tab: every committing binding in scope
// whose `when` admits the selected row. Generated from the keymap, so the menu, the footer and the
// dispatcher cannot describe different worlds.
func (m model) menuOffers() []binding {
	scope := tabScope(m.tab)
	var out []binding
	for _, b := range keymap {
		if !b.commits || (b.scope != scope && b.scope != scopeGlobal) {
			continue
		}
		if b.when != nil && !b.when(m) {
			continue // not applicable to this row: invisible rather than offered and refused
		}
		out = append(out, b)
	}
	return out
}

// menuAccepts reports whether a key is one the open menu is offering. A letter the menu did not
// show does nothing: the point of the prefix is that only what was named can happen next.
func (m model) menuAccepts(k string) bool {
	for _, b := range m.menuOffers() {
		for _, key := range menuKeys(b.keys) {
			if key == k {
				return true
			}
		}
	}
	return false
}

// committingKey reports whether k is a committing binding for the current tab — the keys that are
// inert until the menu is open. Availability is deliberately NOT consulted: a key that commits
// somewhere on this tab is never live bare, or whether a stray press did something would depend on
// which row happened to be selected.
func (m model) committingKey(k string) bool {
	scope := tabScope(m.tab)
	for _, b := range keymap {
		if !b.commits || (b.scope != scope && b.scope != scopeGlobal) {
			continue
		}
		for _, key := range menuKeys(b.keys) {
			if key == k {
				return true
			}
		}
	}
	return false
}

// The availability rules the menu filters on. Each answers "does this action apply to the row in
// front of me", so what cannot be done is not offered — the same thing the hub does for an agent's
// command surface, where an out-of-order verb is invisible rather than refused.

// taskOpen: a finished task cannot be closed again.
func taskOpen(m model) bool {
	t, ok := m.selTask()
	return ok && api.Open(t)
}

// taskHeld: unassign returns a task to the backlog, so it needs somebody holding it.
func taskHeld(m model) bool {
	if id := m.selID(); id != "" {
		_, ok := m.agentOnTask(id)
		return ok
	}
	return false
}

// taskAwaitsVerdict: approve and reject decide a proposal, and a task nobody proposed has nothing
// to decide. A verdict already given is not re-offered either.
func taskAwaitsVerdict(m model) bool {
	t, ok := m.selTask()
	return ok && api.AwaitingVerdict(t)
}

// prDecidable: a PR still being decided takes a verdict or a review; a merged or scrapped one is
// past all three.
func prDecidable(m model) bool {
	id := m.selID()
	for _, p := range m.state.PRs {
		if p.ID == id {
			return api.PROpen(p)
		}
	}
	return false
}

// menuView renders the offers as a centered box: one line per action, the key then what it does.
// Nothing applicable is said out loud rather than shown as an empty frame — "no actions here" is an
// answer, and an empty box reads as a bug.
func (m model) menuView(screenW, screenH int) string {
	offers := m.menuOffers()
	body := dimStyle.Render("nothing to commit on this row")
	if len(offers) > 0 {
		lines := make([]string, len(offers))
		for i, b := range offers {
			lines[i] = stWarn.Render(padTrunc(b.keys, 5)) + " " + b.label(m)
		}
		body = strings.Join(lines, "\n")
	}
	box := modalBorderStyle.Render(
		modalTitleStyle.Render("actions") + "\n" + body + "\n\n" + dimStyle.Render("esc cancels"))
	return lipgloss.Place(screenW, screenH, lipgloss.Center, lipgloss.Center, box)
}
