// package: ui/cli / foreignrows
// type:    command (host CLI)
// job:     print a fleet-wide listing in the labelled groups the TUI's scoped lists
// show — what waits on the user in another repo first, then the repo the
// command was run in, then the rest — and print it flat when nothing waits
// elsewhere.
// limits:  grouping and printing only; the headings are api's, and whether a row
// waits on the user is api's too (-> AgentNeedsUser, PRNeedsUser).
package cli

import (
	"fmt"

	"github.com/flo-at/sindri/internal/api"
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

// listGroupFor places a row by its repo and whether anything is owed on it.
func listGroupFor(project, localTag string, needsUser bool) listGroup {
	switch {
	case localTag == "" || project == localTag:
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

// printGrouped prints what groupedLines assembles.
func printGrouped(rows []listRow) {
	for _, l := range groupedLines(rows) {
		fmt.Println(l)
	}
}
