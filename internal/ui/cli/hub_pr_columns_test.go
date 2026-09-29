package cli

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/ui/table"
)

// TestPRListTakesTheSharedColumnSet is the half of the guard that lives on this side. The drift it
// exists for ran between packages — the TUI's PRs tab gained a "for" column and this listing did not
// — so each front-end pins that its own table still comes from the shared set (-> table.PRList).
func TestPRListTakesTheSharedColumnSet(t *testing.T) {
	shared := table.PRList(13)
	if len(prListTable) != len(shared)+1 {
		t.Fatalf("`pr list` has %d columns, want the shared %d plus its own tail", len(prListTable), len(shared))
	}
	for i, want := range shared {
		if prListTable[i].Label != want.Label {
			t.Errorf("column %d is %q, want the shared %q", i, prListTable[i].Label, want.Label)
		}
	}
	if last := prListTable[len(prListTable)-1].Label; last != "waiting on you" {
		t.Errorf("the tail is %q, want this medium's own trailing column", last)
	}
}

// TestTheStatusAgeReachesTheListing is the reported symptom: how long a PR has held its status was
// on the TUI and nowhere here, though StatusChangedAt was already on the wire. Asserted on the
// header, since that is what a reader looks for before they look for the value.
func TestTheStatusAgeReachesTheListing(t *testing.T) {
	header := prListTable.Header()
	for _, label := range []string{"status", "for", "age", "try"} {
		if !strings.Contains(header, label) {
			t.Errorf("the listing's header %q has no %q column", strings.TrimSpace(header), label)
		}
	}
}
