// package: api / foreign
// type:    logic (display vocabulary)
// job:     the words a list uses when it holds rows from another repo — the heading
// over the rows that are only there because they wait on the user, and the
// heading over the rows of the repo in view.
// limits:  wording only; which rows a list admits is its own (-> ui/tui items.go),
// and the counts on the tab handles are the hub's (-> Section.Attention).
package api

import "fmt"

// ForeignAttentionHeading labels the rows a list carries from another repo, which are on screen for
// one reason: they wait on the user. It states n because the same rows are counted by the "(N!)"
// badge on the tab handle, which says how many and not which — one claim in two renderings, so they
// have to be read together rather than matched by hand.
func ForeignAttentionHeading(n int) string {
	return fmt.Sprintf("Needing attention in other repos (%d):", n)
}

// LocalHeading labels the repo in view, beneath the foreign rows. Shown only when there ARE foreign
// rows: a permanent heading taxes every ordinary glance to explain an occasional one.
const LocalHeading = "Local:"

// OtherReposHeading labels the rest of the fleet, in a listing that is fleet-wide to begin with —
// `sindri agent list` and `sindri pr list` show every repo, so what is left once the waiting rows
// are lifted out is not local and must not be labelled as though it were.
const OtherReposHeading = "Other repos:"
