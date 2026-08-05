// package: hub / sections
// type:    logic (board badge counts)
// job:     BoardState's badge counts cross the wire, so they are now its own methods
// in internal/api. PROpen stays visible here under its existing name.
// limits:  the section list + titles live in hub/commands; counting lives in internal/api.
package hub

import "github.com/flo-at/sindri/internal/api"

// PROpen reports whether a PR is still open — in neither terminal state (merged or
// scrapped). Exported because a UI that narrows the board to one repo has to apply the
// SAME open-ness rule to its subset that OpenPRCount applies to the whole fleet; if it
// reimplemented the rule, a tab badge could disagree with the list beneath it.
var PROpen = api.PROpen
