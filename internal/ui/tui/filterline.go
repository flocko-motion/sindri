// package: tui / filterline
// type:    ui (the line saying how a list is narrowed)
// job:     state every axis narrowing the active tab's list, and name the key that clears them —
// a list showing four rows cannot otherwise be told from a slice of forty.
// limits:  the wording and the rule for when it is shown; placing it in the list is the
// assembler's (-> rowsections.go), and clearing is onKey's (-> clearFilters).
package tui

import (
	"strings"

	"github.com/flo-at/sindri/internal/api"
)

// narrowing is one axis in force on a tab's list, in the order it is read.
type narrowing struct {
	label   string // what it says: "filter: unread", "to dvalin", "repo sindri"
	dflt    bool   // it is the value the tab opens with, so it alone raises no line
	applies bool   // this tab has this axis at all
}

// filterLine names every axis narrowing this tab's list, "" for the ordinary view — shown only when
// something is non-default, and then naming every axis, defaults included, since the one that raised
// the line is rarely the only one hiding rows.
func (m model) filterLine() string {
	axes := m.narrowings()
	ordinary := true
	for _, a := range axes {
		if a.applies && !a.dflt {
			ordinary = false
		}
	}
	if ordinary {
		return ""
	}
	var parts []string
	for _, a := range axes {
		if a.applies {
			parts = append(parts, a.label)
		}
	}
	line := stWarn.Render(strings.Join(parts, " · "))
	if note := m.filterNote(); note != "" {
		line += stWarn.Render(" — " + note)
	}
	return line + dimStyle.Render("  ·  "+keyClearFilters+" clears")
}

// filterNote is a promise the line owes beyond its axes (-> mailShortfall). Mail alone has one: it
// is the only tab whose rows are a window rather than the whole set.
func (m model) filterNote() string {
	if m.tab == 6 {
		return m.mailShortfall()
	}
	return ""
}

// narrowings is what each tab can be narrowed by. Tasks is repo-scoped regardless of the toggle, so
// it has no scope axis; Repos and the meeting have no filters at all.
func (m model) narrowings() []narrowing {
	scope := narrowing{label: m.scopeLabel(), dflt: m.scopeRepo, applies: true}
	switch m.tab {
	case 0:
		return []narrowing{
			{label: "filter: " + string(m.filter), dflt: m.filter == api.FilterActive, applies: true},
			{label: "search: " + m.taskSearch, dflt: m.taskSearch == "", applies: m.taskSearch != ""},
		}
	case 1:
		return []narrowing{scope}
	case 2:
		return []narrowing{
			{label: "filter: " + string(m.prFilter), dflt: m.prFilter == api.PRFilterActive, applies: true},
			scope,
		}
	case 5:
		return []narrowing{
			{label: "filter: " + string(m.runFilter), dflt: m.runFilter == api.RunFilterActive, applies: true},
			scope,
		}
	case 6:
		return []narrowing{
			{label: "filter: " + string(m.mailFilter), dflt: m.mailFilter == api.MailFilters[0], applies: true},
			{label: "to " + m.mailAgent, dflt: m.mailAgent == "", applies: m.mailAgent != ""},
			scope,
		}
	}
	return nil
}

// scopeLabel says which repos the list admits. Named even when it is the default, since it hides
// rows either way and the line only appears when something else already raised it.
func (m model) scopeLabel() string {
	if !m.scopeRepo {
		return "all repos"
	}
	if name, _ := m.currentRepo(); name != "" {
		return "repo " + name
	}
	return "this repo"
}

// clearFilters puts every axis back to what its tab opens with — the DEFAULT rather than the
// widest, since "all" is itself a filter that would leave the line, and the promise, unkept.
func (m *model) clearFilters() {
	if m.filterLine() == "" {
		return // nothing narrowed: leave esc to mean nothing rather than reset the scope silently
	}
	m.filter, m.prFilter, m.runFilter = api.FilterActive, api.PRFilterActive, api.RunFilterActive
	m.mailFilter, m.mailAgent, m.mailPromised = api.MailFilters[0], "", 0
	m.taskSearch = ""
	m.scopeRepo = true
	m.cursor[m.tab] = 0
	m.flash = "filters cleared on every tab" // reaches further than the line above THIS tab's rows
}
