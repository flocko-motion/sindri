// package: ui/theme / age
// type:    rendering (shared presentation primitives)
// job:     render an RFC3339 timestamp as a short age ("10m", "2d") for a list column, and as a
// full local datetime for a detail pane — one spelling for both front-ends.
// limits:  formatting only; the timestamps come from the hub.
package theme

import (
	"fmt"
	"time"
)

// Age renders how long ago t was, in the width of a list column: "just now" under a minute, then
// minutes, hours, days, weeks. One unit only — a column is read at a glance, and "2d" answers the
// question "is this from today?" as well as "2d 4h" does. Empty for a timestamp the source never
// gave, since a wrong age is worse than none.
func Age(rfc3339 string) string {
	t, ok := parseStamp(rfc3339)
	if !ok {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < 0: // a clock difference, not a task from the future
		return "now"
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	default:
		return fmt.Sprintf("%dw", int(d.Hours())/(24*7))
	}
}

// Stamp renders the exact moment for a detail pane, in the reader's own timezone — the answer the
// column's rounded age deliberately does not give. Empty when unknown, like Age.
func Stamp(rfc3339 string) string {
	t, ok := parseStamp(rfc3339)
	if !ok {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04")
}

// parseStamp accepts what the sources actually write: RFC3339, and the date-only form a tracker may
// give. Anything else is treated as unknown rather than guessed at.
func parseStamp(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		// A zero time.Time formats as a valid RFC3339 year 1, which would render as ~105000w rather
		// than as the "no answer" it is.
		if t, err := time.Parse(layout, s); err == nil && t.Year() > 2000 {
			return t, true
		}
	}
	return time.Time{}, false
}
