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
