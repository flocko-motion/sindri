// package: lint / cap
// type:    logic (output budget)
// job:     bound how many findings a run prints, so a repo-wide failure arrives as a
// list someone can work off rather than a wall that scrolls away.
// limits:  counts and writes the closing note; each linter decides what one finding is.
package lint

import (
	"fmt"
	"io"
)

// DefaultLimit is how many findings a run prints before summarising the rest — about one screen,
// enough to fix in a pass. The whole list is `--limit 0`.
const DefaultLimit = 15

// Cap is one budget for a WHOLE run, not per linter: six linters each printing "only" their share
// is exactly the wall this exists to prevent.
type Cap struct {
	max    int
	shown  int
	hidden int
}

// NewCap bounds a run to max findings; max <= 0 means print everything.
func NewCap(max int) *Cap { return &Cap{max: max} }

// Allow reports whether the next finding should be printed, and counts it either way. A nil Cap
// allows everything, so a caller with no budget needs no special case.
func (c *Cap) Allow() bool {
	if c == nil || c.max <= 0 {
		return true
	}
	if c.shown < c.max {
		c.shown++
		return true
	}
	c.hidden++
	return false
}

// Note reports what was withheld and how to see it, or nothing when everything fit. Findings the
// caller never printed have to be counted here, or a capped run reads as a clean one.
func (c *Cap) Note(w io.Writer) {
	if c == nil || c.hidden == 0 {
		return
	}
	fmt.Fprintf(w, "… and %d more finding(s) not shown — `--limit <n>` prints more, `--limit 0` all.\n", c.hidden)
}

// Hidden is how many findings went unprinted, for a caller that words its own note.
func (c *Cap) Hidden() int {
	if c == nil {
		return 0
	}
	return c.hidden
}
