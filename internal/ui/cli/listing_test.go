package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/table"
)

// listing is the fixture both listings reduce to: a local row, a foreign one waiting on the user,
// another foreign one that is not, in the repo order the sort hands over.
func listing() []listRow {
	return []listRow{
		{"sin  eitri  working", listGroupFor("sin", "sin", false)},
		{"oth  thrain full", listGroupFor("oth", "sin", true)},
		{"oth  gloin  working", listGroupFor("oth", "sin", false)},
	}
}

// TestAForeignRowWaitingOnYouOpensTheListing: the CLI listing is fleet-wide, so a row from elsewhere
// is placed only by a repo column a reader skims. Under a heading, and first, it says what it is —
// the same reading the Agents and PRs tabs teach.
func TestAForeignRowWaitingOnYouOpensTheListing(t *testing.T) {
	got := groupedLines(listing())
	if len(got) == 0 {
		t.Fatal("no lines")
	}
	if got[0] != api.ForeignAttentionHeading(1) {
		t.Errorf("the listing should open with the foreign heading and its count, got %q", got[0])
	}
	if !strings.Contains(got[1], "thrain") {
		t.Errorf("the waiting row belongs under that heading, got %q", got[1])
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "\n\n"+api.LocalHeading+"\n") {
		t.Errorf("the local rows need a blank line and a heading of their own:\n%s", joined)
	}
	// The rest of the fleet is still shown, and is not labelled local — in a fleet-wide listing it
	// is neither, and a heading that claimed otherwise would be the misreading this fixes.
	if !strings.Contains(joined, "\n\n"+api.OtherReposHeading+"\n") {
		t.Errorf("the rows from elsewhere that ask nothing need their own heading:\n%s", joined)
	}
	if strings.Index(joined, "gloin") < strings.Index(joined, "eitri") {
		t.Errorf("local comes before the rest of the fleet:\n%s", joined)
	}
}

// TestAnOrdinaryListingIsUntouched: headings only earn their place when something waits elsewhere.
// Otherwise the listing must be exactly what it was, in exactly the order the sort gave it.
func TestAnOrdinaryListingIsUntouched(t *testing.T) {
	rows := []listRow{
		{"sin  eitri  working", listGroupFor("sin", "sin", false)},
		{"oth  gloin  working", listGroupFor("oth", "sin", false)},
		{"sin  dvalin down", listGroupFor("sin", "sin", false)},
	}
	got := groupedLines(rows)
	want := []string{"sin  eitri  working", "oth  gloin  working", "sin  dvalin down"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("a listing with nothing waiting elsewhere should be flat and in order, got:\n%s", strings.Join(got, "\n"))
	}
}

// TestOutsideARegisteredRepoNothingIsForeign: with no local repo there is nothing for a row to be
// foreign to, so the listing stays flat rather than grouping every row as somebody else's.
func TestOutsideARegisteredRepoNothingIsForeign(t *testing.T) {
	rows := []listRow{
		{"sin  eitri  working", listGroupFor("sin", "", false)},
		{"oth  thrain full", listGroupFor("oth", "", true)},
	}
	for _, l := range groupedLines(rows) {
		if strings.HasSuffix(l, ":") || l == "" {
			t.Errorf("nothing to group against, yet the listing is sectioned: %q", l)
		}
	}
}

// TestAnEmptySectionPrintsNoHeading: a heading over nothing reads as a group whose rows failed to
// print. Every repo's agents stuck at once is the case that produces it.
func TestAnEmptySectionPrintsNoHeading(t *testing.T) {
	rows := []listRow{{"oth  thrain full", listGroupFor("oth", "sin", true)}}
	got := groupedLines(rows)
	if len(got) != 2 || got[0] != api.ForeignAttentionHeading(1) {
		t.Fatalf("want the foreign heading and its one row, got %q", got)
	}
	if strings.Contains(strings.Join(got, "\n"), api.LocalHeading) {
		t.Errorf("no local rows, so no local heading: %q", got)
	}
}

// TestGlobalProjectIsNeverForeign: GlobalProject belongs to no repo, so it must group as local even
// when idle in a different repo's listing — unlike an ordinary foreign row, which needs
// api.AgentNeedsUser (or its PR/mail equivalent) to earn a place at all.
func TestGlobalProjectIsNeverForeign(t *testing.T) {
	if got := listGroupFor(api.GlobalProject, "sin", false); got != groupLocal {
		t.Errorf("listGroupFor(GlobalProject, ...) = %v, want groupLocal", got)
	}
}

// cliTables is every column layout the CLI lists through, by the command that prints it.
var cliTables = map[string]table.Table{
	"agent list": agentListTable,
	"pr list":    prListTable,
	"task list":  taskListTable,
	"mail list":  mailListTable,
	"run list":   runListTable,
	"repo list":  repoListTable,
}

// TestEveryCLIListingLabelsItsColumns: the same confusion and the same fix as the TUI's, and the
// listing a user reads in a terminal is the one where the recipient and the sender sat adjacent and
// unnamed. A label wider than its column would bleed into the next and put the header out of step
// with the rows it names.
func TestEveryCLIListingLabelsItsColumns(t *testing.T) {
	for name, tbl := range cliTables {
		if len(tbl) == 0 {
			t.Errorf("%s has no columns", name)
		}
		for i, c := range tbl {
			if c.Width == 0 {
				if i != len(tbl)-1 {
					t.Errorf("%s column %d (%q) has no width but is not the last", name, i, c.Label)
				}
				continue
			}
			if c.Label == "" {
				t.Errorf("%s column %d is unlabelled", name, i)
			}
			if w := ansi.StringWidth(c.Label); w > c.Width {
				t.Errorf("%s column %d: label %q is %d cells in a column of %d", name, i, c.Label, w, c.Width)
			}
		}
	}
}

// TestMailListReadsFromThenTo: the order that prompted this. `to` before `from` is the reverse of how
// mail is read anywhere else, and the two columns are adjacent, so the reader has nothing to correct
// against — labelling that order would only have made the backwardness legible.
func TestMailListReadsFromThenTo(t *testing.T) {
	from, to := -1, -1
	for i, c := range mailListTable {
		switch c.Label {
		case "from":
			from = i
		case "to":
			to = i
		}
	}
	if from < 0 || to < 0 {
		t.Fatal("`mail list` must label its sender and its recipient")
	}
	if from > to {
		t.Error("sender belongs before recipient, the order mail is read in everywhere else")
	}
}

// TestTheHeaderIsLaidOutByTheRowsOwnColumns: one layout does both jobs, which is the whole guarantee
// — a header assembled from widths of its own is aligned the day it is written and drifts after.
func TestTheHeaderIsLaidOutByTheRowsOwnColumns(t *testing.T) {
	row := mailListTable.Line(
		table.Cell{Text: "42"},
		table.Cell{Text: "sindri"},
		table.Cell{Text: "user"},
		table.Cell{Text: "dvalin"},
		table.Cell{Text: "unread"},
		table.Cell{Text: "2d"},
		table.Cell{Text: "read the brief again"},
	)
	header := mailListTable.Header()
	// "from" is the sender's column: the label and the value start in the same cell.
	if strings.Index(header, "from") != strings.Index(row, "user") {
		t.Errorf("the from label does not sit over the sender:\n%q\n%q", header, row)
	}
	if strings.Index(header, "to") != strings.Index(row, "dvalin") {
		t.Errorf("the to label does not sit over the recipient:\n%q\n%q", header, row)
	}
}
