// package: ui/cli / listing
// type:    command (host CLI)
// job:     print a listing's lines that are not rows — the column labels over it, and
// the group headings a fleet-wide listing needs when something waits on the
// user in another repo: those rows first, then the repo the command was run
// in, then the rest.
// limits:  labelling and printing only; the widths are the listing's table (-> ui/table),
// the group headings are api's, and whether a row waits on the user is api's
// too (-> AgentNeedsUser, PRNeedsUser).
package cli

import (
	"fmt"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/table"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// listGroup is which section of a listing a row belongs in.
type listGroup int

const (
	groupLocal            listGroup = iota // the repo the command was run in
	groupOther                             // another repo, asking nothing of the user
	groupForeignAttention                  // another repo, and it waits on the user
)

// listRow is one printed line with the group it belongs in.
type listRow struct {
	line  string
	group listGroup
}

// localProject is the tag of the repo the command was run in, "" when the cwd is not a registered
// one. Then there is no local repo for a row to be foreign to, and the listing stays flat.
func localProject(projects []api.Project) string {
	root, err := repoRoot()
	if err != nil {
		return ""
	}
	for _, p := range projects {
		if p.Path == root {
			return p.Tag
		}
	}
	return ""
}

// listGroupFor places a row by its repo and whether anything is owed on it. GlobalProject belongs to
// no repo, so it is never foreign to one.
func listGroupFor(project, localTag string, needsUser bool) listGroup {
	switch {
	case localTag == "" || project == localTag || project == api.GlobalProject:
		return groupLocal
	case needsUser:
		return groupForeignAttention
	}
	return groupOther
}

// groupedLines is the listing as it should be printed: under headings when any row waits on the user
// in another repo, and in the given order when none does. Foreign first, because needing the user is
// the only reason such a row is singled out; flat otherwise, since a permanent heading would tax
// every ordinary listing to explain an occasional one.
func groupedLines(rows []listRow) []string {
	byGroup := map[listGroup][]string{}
	for _, r := range rows {
		byGroup[r.group] = append(byGroup[r.group], r.line)
	}
	foreign := byGroup[groupForeignAttention]
	if len(foreign) == 0 {
		out := make([]string, 0, len(rows))
		for _, r := range rows {
			out = append(out, r.line)
		}
		return out
	}
	sections := []struct {
		heading string
		lines   []string
	}{
		{api.ForeignAttentionHeading(len(foreign)), foreign},
		{api.LocalHeading, byGroup[groupLocal]},
		{api.OtherReposHeading, byGroup[groupOther]},
	}
	var out []string
	for _, s := range sections {
		if len(s.lines) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, "") // the blank line that makes the sections read as sections
		}
		out = append(out, s.heading)
		out = append(out, s.lines...)
	}
	return out
}

// printListing prints the column labels over the rows, then the rows themselves, grouped where any
// of them waits on the user in another repo. Nothing at all when there are no rows: labels over an
// empty table explain nothing, and each command's own closing line says why it is empty.
func printListing(t table.Table, rows []listRow) {
	if len(rows) == 0 {
		return
	}
	fmt.Println(theme.Dim().Render(t.Header()))
	for _, l := range groupedLines(rows) {
		fmt.Println(l)
	}
}

// printRows is printListing for a listing with no repo grouping to make: the labels, then the rows.
func printRows(t table.Table, lines []string) {
	rows := make([]listRow, len(lines))
	for i, l := range lines {
		rows[i] = listRow{l, groupLocal}
	}
	printListing(t, rows)
}

// DefaultListLimit bounds every history listing the same way defaulting to "active" does: a filter
// alone still lets a listing grow without bound as the fleet grows (sd-4be9f8) — the bound wants a
// count too.
const DefaultListLimit = 50

// limitNotice is the line a capped listing prints once --limit (or -n) cuts something, naming the
// count and how to see the rest. Silent truncation would read as a complete answer, which for a
// listing is a wrong one — the same principle gitcmd.go's capLines states for an agent reading a
// diff applies here for a person reading a list.
func limitNotice(noun string, shown, matched int) string {
	if shown >= matched {
		return ""
	}
	return fmt.Sprintf("(%d of %d %s(s) shown — raise with --limit, or --limit 0 for all)\n", shown, matched, noun)
}

// capHead keeps the first limit items (all of them when limit <= 0), plus how many there were
// before that. Pick this over capTail when items already sort newest (or most important) first, so
// the kept prefix is the newest.
func capHead[T any](items []T, limit int) (kept []T, matched int) {
	matched = len(items)
	if limit <= 0 || matched <= limit {
		return items, matched
	}
	return items[:limit], matched
}

// capTail is capHead for a source that sorts oldest first: it keeps the last limit items, so the
// kept suffix is still the newest.
func capTail[T any](items []T, limit int) (kept []T, matched int) {
	matched = len(items)
	if limit <= 0 || matched <= limit {
		return items, matched
	}
	return items[matched-limit:], matched
}
