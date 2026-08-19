// package: tui / component_menu
// type:    ui component (the space-prefix action menu)
// job:     show the committing actions for what is selected, into the footer's own two rows
// (-> menuFooter) rather than over the screen, and make their letters live while open.
// limits:  chrome and the offer list; the actions themselves stay in onKey, and which
// bindings commit is declared in the keymap (-> keys.go).
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

// menuOffers is every committing binding in scope whose `when` admits the selected row, generated
// from the keymap so the menu and the dispatcher cannot describe different worlds.
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

// committingKey reports whether k commits somewhere on this tab — inert until the menu is open.
// Availability is not consulted (a key that commits is never live bare, whichever row is
// selected), except that disarming a clear stays bare: cancelling a destructive action isn't one.
func (m model) committingKey(k string) bool {
	// Arming a clear opens a confirm, so it commits; disarming is not itself destructive — cancelling
	// one is safe, as its own onKey branch already argues — so it stays reachable bare even though
	// both share this key and scope.
	if k == keyClearCtx && m.tab == 1 {
		if a, ok := m.selAgent(); ok && a.ClearArmed {
			return false
		}
	}
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

// The availability rules the menu filters on: each answers "does this apply to this row", so what
// cannot be done is not offered rather than offered and refused.

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

// mailAttachable: attach needs a live agent on one side of the message — hub/user/reviewer are
// never one, so a hub notice to the user has nobody to reach (-> mailAttachTarget).
func mailAttachable(m model) bool {
	_, ok := m.mailAttachTarget()
	return ok
}

// menuEntryStyle is white, the opposite of the footer's uniform dim — the only sign the menu is
// open now that it no longer takes the screen.
var menuEntryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("231"))

// menuFooter renders the offers into the footer's two rows in place of the ordinary hints. Nothing
// applicable is said out loud rather than shown as an empty row — "no actions here" is an answer.
// In practice this branch has no way in from a real tab any more: `E config` is scopeGlobal and
// commits (sd-6d0ff2), so it fills every tab's menu on its own (-> TestGlobalConfigFillsEveryTabsMenu).
// Kept correct regardless — a `when` narrowing that binding, or a new global one added without
// commits, would make this reachable again.
func (m model) menuFooter(width int) string {
	offers := m.menuOffers()
	if len(offers) == 0 {
		return dimStyle.Render(padTrunc("nothing to commit on this row", width)) + "\n" + strings.Repeat(" ", width)
	}
	entries := make([]string, len(offers))
	for i, b := range offers {
		entries[i] = stWarn.Render(b.keys) + " " + menuEntryStyle.Render(b.label(m))
	}
	row1, row2 := menuFooterRows(entries, width)
	return padTrunc(row1, width) + "\n" + padTrunc(row2, width)
}

// menuFooterRows packs entries across two rows, breaking only between entries, ANSI-aware and
// cell-accurate (entries carry colour). Ends row 2 with "…", no count, when they still overflow.
func menuFooterRows(entries []string, width int) (row1, row2 string) {
	var rows [2]string
	i := 0
	for r := 0; r < len(rows) && i < len(entries); r++ {
		line := ""
		for i < len(entries) {
			candidate := entries[i]
			if line != "" {
				candidate = line + " · " + entries[i]
			}
			if ansi.StringWidth(candidate) > width {
				if line == "" { // this one entry alone doesn't fit even on an empty row: say part of
					// it rather than nothing, so the row still names something instead of going blank.
					line = ansi.Truncate(entries[i], width, "…")
					i++
				}
				break
			}
			line = candidate
			i++
		}
		rows[r] = line
	}
	if i < len(entries) {
		rows[1] = withEllipsis(rows[1], width)
	}
	return rows[0], rows[1]
}

// withEllipsis trims line back to the last whole entry so a trailing "…" still fits within width —
// the only sign the offers ran past two rows.
func withEllipsis(line string, width int) string {
	for line != "" && ansi.StringWidth(line)+2 > width {
		idx := strings.LastIndex(line, " · ")
		if idx < 0 {
			line = ""
			break
		}
		line = line[:idx]
	}
	if line == "" {
		return ansi.Truncate("…", width, "")
	}
	return line + " …"
}
