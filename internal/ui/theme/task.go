// package: ui/theme / task
// type:    logic (shared presentation primitives)
// job:     the priority/state display words both front-ends render, and the
// dial-in client summary — so the CLI and the TUI agree on the same words
// instead of each inventing its own.
// limits:  string derivation only; the data (a task's priority/status, an
// agent's client list) comes from internal/api.
package theme

import (
	"fmt"
	"strings"

	"github.com/flo-at/sindri/internal/api"
)

// PriorityLabel maps td's P0…P4 priority codes to readable words for display (sorting
// still uses the codes). Shared by the CLI and the TUI so they agree.
func PriorityLabel(p string) string {
	switch p {
	case "P0":
		return "critical"
	case "P1":
		return "high"
	case "P2":
		return "mid"
	case "P3":
		return "low"
	case "P4":
		return "none" // "came in unrated" — GitHub issues import here by default
	case "":
		return "-"
	default:
		return p
	}
}

// PriorityCode maps a readable word to td's P-code (the inverse of PriorityLabel). A
// value already in P-code form passes through.
func PriorityCode(word string) string {
	switch word {
	case "critical":
		return "P0"
	case "high":
		return "P1"
	case "mid", "medium":
		return "P2"
	case "low":
		return "P3"
	case "none", "trivial", "minor": // trivial/minor kept as back-compat input aliases
		return "P4"
	default:
		return word
	}
}

// PriorityWords are the assignable priorities, highest first (for choice menus).
var PriorityWords = []string{"critical", "high", "mid", "low", "none"}

// Plural renders a counted noun ("1 task", "3 tasks"), so a confirmation line and the modal it
// mirrors count the same way.
func Plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// PriorityScopeNote states what carrying a rating below this task would actually do, "" when there is
// nothing below to carry it to. Shared, because a user offered the wider scope has to be told which of
// the two answers applies (-> api.PriorityEffect): under an open parent a child's rating only orders
// the package's subtasks, and a menu silent about that reads as releasing work it merely reordered.
func PriorityScopeNote(c api.PriorityCascade) string {
	one := c.Children == 1
	switch {
	case c.Children == 0:
		return ""
	case c.Independent && one:
		return "the 1 task below will be claimed on its own — a priority is what makes it claimable"
	case c.Independent:
		return Plural(c.Children, "task", "tasks") +
			" below will be claimed on their own — a priority is what makes each claimable"
	case one:
		return "the 1 task below comes with this package — rating it sets the ORDER it is worked in, " +
			"not whether it is released"
	}
	return Plural(c.Children, "task", "tasks") +
		" below come with this package — rating them sets the ORDER they're worked, not whether they're released"
}

// PriorityScopeLabels are the menu labels for api.PriorityScopes, in that order, each carrying the
// count it would touch so the choice is made against the real number rather than a category.
func PriorityScopeLabels(c api.PriorityCascade) []string {
	return []string{
		"this task only",
		"+ the " + Plural(c.Unrated, "task", "tasks") + " below with no priority set",
		"+ all " + Plural(c.Children, "task", "tasks") + " below (overwrite)",
	}
}

// StateLabel maps a task status to a short, fixed-ish word for compact display (so the
// column doesn't need room for "in_progress"). Shared by CLI and TUI.
func StateLabel(s string) string {
	switch s {
	case "in_progress":
		return "active"
	case "in_review":
		return "review"
	case "closed":
		return "done"
	case "approved":
		return "appr"
	default:
		return s // open, merged, …
	}
}

// FormatClients is shared by CLI `agent info` and the TUI detail view, so both read alike.
func FormatClients(cs []api.ClientView) string {
	if len(cs) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "clients:   %d attached\n", len(cs))
	for _, c := range cs {
		mode := "read-write"
		if c.ReadOnly {
			mode = "read-only"
		}
		fmt.Fprintf(&b, "  %s  %dx%d  %s\n", c.TTY, c.Width, c.Height, mode)
	}
	return b.String()
}

// FormatNext renders the assignment explanation, shared by `task next` and the TUI so both give the
// same account. Claimable rows first — the answer to "what happens next" is at the top.
func FormatNext(x api.NextExplain) string {
	var b strings.Builder
	switch {
	case x.AgentNote != "":
		fmt.Fprintf(&b, "%s takes nothing right now: %s\n\n", x.Agent, x.AgentNote)
	case x.Pick != nil:
		fmt.Fprintf(&b, "next: %s  %s  (%s)\n\n", x.Pick.ID, x.Pick.Title, x.Pick.Why)
	default:
		fmt.Fprintf(&b, "next: nothing — no open task can be handed to anyone\n\n")
	}
	for _, pass := range []bool{true, false} {
		for _, t := range x.Tasks {
			if t.Claimable() != pass {
				continue
			}
			fmt.Fprintf(&b, "%-12s %-8s %-42s %s", t.ID, PriorityLabel(t.Priority), t.Why, t.Title)
			if t.Note != "" {
				fmt.Fprintf(&b, "\n%-12s %-8s %s", "", "", t.Note)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// MemoryDefaultLabel names an agent's unset RAM limit with the figure actually in force, shared so
// the CLI and the TUI cannot print different defaults. Empty (no runtime wired) says so rather than
// inventing a number.
func MemoryDefaultLabel(dflt string) string {
	if strings.TrimSpace(dflt) == "" {
		return "(hub default)"
	}
	return dflt + " (default)"
}
