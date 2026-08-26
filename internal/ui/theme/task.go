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
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/flo-at/sindri/internal/api"
)

// PriorityLabel maps td's P0…P4 priority codes to readable words for display (sorting
// still uses the codes). Shared by the CLI and the TUI so they agree.
func PriorityLabel(p string) string {
	if word := api.PriorityLabel(p); word != "" {
		return word
	}
	if p == "" {
		return "-" // unrated, which is a different fact from P4 ("none", the lowest rating)
	}
	return p
}

// PriorityCode maps a readable word to td's P-code (the inverse of PriorityLabel). A value already
// in P-code form passes through, and so does anything unrecognised — this is the lenient front-end
// door onto api's table, which is where the vocabulary itself lives.
func PriorityCode(word string) string {
	if code, ok := api.ParsePriority(word); ok {
		return code
	}
	return word
}

// PriorityWords are the assignable priorities, highest first (for choice menus) — api's list, not a
// copy: a menu offering a word the hub cannot parse is a bug nobody sees until someone picks it.
var PriorityWords = api.PriorityWords

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

// ApprovalLabel is the word an approval state is shown as. "pending" names a state; "unapproved"
// names the ACTION that is missing, which is what a reader wants when a task is sitting still and
// they are working out why. The stored value is untouched — every predicate branches on it — so the
// change is here, where presentation lives, and both front-ends therefore say the same word.
func ApprovalLabel(a string) string {
	if a == "pending" {
		return "unapproved"
	}
	return a // "", "approved", "rejected" — each already says what it is
}

// TaskRelationLabel is the word for an agent's hold on a task, shared with the glyph the marker
// column draws from the same relation — a mark and its explanation must not drift apart.
func TaskRelationLabel(rel api.TaskRelation) string {
	switch rel {
	case api.TaskHolding:
		return "holds this hierarchy"
	case api.TaskSubmitted:
		return "submitted it, awaiting a verdict"
	}
	return "working this task"
}

// AttemptCell renders which submission is standing, blank for the first: numbering the majority that
// land first would bury the one on its fourth, which is what a reader scans for (-> table.PRList).
func AttemptCell(n int) string {
	if n < 2 {
		return ""
	}
	return "×" + strconv.Itoa(n)
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

// reasonW is the width the standing column is padded to — wide enough for the longest reason
// either pool can give, so the titles beside them line up.
const reasonW = 42

// padCell pads s to w display CELLS: %-42s pads by bytes, and a dash is three of them, so every
// reason containing one pulled its title two columns left of the row above.
func padCell(s string, w int) string {
	if pad := w - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// FormatNext renders the assignment explanation for the CLI and the TUI alike, claimable rows
// first. It renders the half the role's pool fills — tasks, PRs, or the sentence saying a role is
// served from neither, which an empty list would report as "no work".
func FormatNext(x api.NextExplain) string {
	var b strings.Builder
	if x.RoleNote != "" {
		fmt.Fprintf(&b, "%s: %s\n", x.Role, x.RoleNote)
		return b.String()
	}
	if x.Role == "reviewer" {
		return formatNextReview(x)
	}
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
			fmt.Fprintf(&b, "%s %s %s %s", padCell(t.ID, 12), padCell(PriorityLabel(t.Priority), 8),
				padCell(string(t.Why), reasonW), t.Title)
			if t.Note != "" {
				fmt.Fprintf(&b, "\n%s %s %s", padCell("", 12), padCell("", 8), t.Note)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// formatNextReview is the reviewer's half: the review that would be picked up, then where every
// other PR stands.
func formatNextReview(x api.NextExplain) string {
	var b strings.Builder
	switch {
	case x.AgentNote != "":
		fmt.Fprintf(&b, "%s takes nothing right now: %s\n\n", x.Agent, x.AgentNote)
	case x.PickPR != nil:
		fmt.Fprintf(&b, "next: %s  %s  (%s)\n\n", x.PickPR.ID, x.PickPR.Title, x.PickPR.Why)
	case len(x.PRs) == 0:
		fmt.Fprintf(&b, "next: nothing — there are no open PRs at all\n\n")
	default:
		fmt.Fprintf(&b, "next: nothing — no PR is waiting on a review\n\n")
	}
	for _, pass := range []bool{true, false} {
		for _, p := range x.PRs {
			if p.Reviewable() != pass {
				continue
			}
			fmt.Fprintf(&b, "%s %s %s", padCell(p.ID, 20), padCell(string(p.Why), reasonW), p.Title)
			if p.Note != "" {
				fmt.Fprintf(&b, "\n%s %s", padCell("", 20), p.Note)
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
