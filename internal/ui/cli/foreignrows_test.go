package cli

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
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
