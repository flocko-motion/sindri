// package: ui/theme / age
// type:    rendering (shared presentation primitives)
// job:     render an RFC3339 timestamp as a short age ("10m") for a list column, as prose for a
// sentence ("just now", "3m ago", "for under a minute"), and as a full local datetime
// for a detail pane — one spelling for both front-ends.
// limits:  formatting only; the timestamps come from the hub.
package theme

import (
	"fmt"
	"time"
)

// Unknown is what a timestamp no source could give renders as. Said outright rather than left
// blank: an empty cell reads as a rendering gap, and inventing an age would be worse than either.
const Unknown = "n/a"

// Age renders how long ago t was, in the width of a list column: "now" under a minute, then minutes,
// hours, days, weeks. One unit only — a column is read at a glance, and "2d" answers the question
// "is this from today?" as well as "2d 4h" does. For a sentence rather than a column, use Ago.
func Age(rfc3339 string) string {
	t, ok := parseStamp(rfc3339)
	if !ok {
		return Unknown
	}
	return ageOf(time.Since(t))
}

// Ago renders a timestamp as PROSE — "just now", "3m ago" — for a line that reads as a sentence.
// Appending " ago" to Age is what four call sites each did, and under a minute Age answers with a
// whole phrase rather than a magnitude, so every one of them read "now ago".
func Ago(rfc3339 string) string {
	t, ok := parseStamp(rfc3339)
	if !ok {
		return Unknown // no moment, so no "ago" to hang on it either
	}
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	return ageOf(d) + " ago"
}

// Held renders how long a state has been worn, for a phrase like "open for 3d". Under a minute it
// spells the duration out, because "for now" is real English meaning the opposite — for the time
// being — where "for under a minute" can only be read as the age it is.
func Held(rfc3339 string) string {
	t, ok := parseStamp(rfc3339)
	if !ok {
		return Unknown
	}
	d := time.Since(t)
	if d < time.Minute {
		return "under a minute"
	}
	return ageOf(d)
}

// ageOf is the magnitude the three forms share, so the boundary between "a number" and "a phrase" is
// decided once. Callers have already ruled out an unparseable stamp.
func ageOf(d time.Duration) string {
	switch {
	case d < time.Minute: // negative included: a clock difference, not a moment from the future
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
// column's rounded age deliberately does not give. Unknown when the source has none, like Age.
func Stamp(rfc3339 string) string {
	t, ok := parseStamp(rfc3339)
	if !ok {
		return Unknown
	}
	return t.Local().Format("2006-01-02 15:04")
}

// When is the detail pane's form of a timestamp: the exact local moment with its age beside it,
// "n/a" where a source gives none. One spelling, because a detail line pairing created with changed
// is unreadable if the two are written differently, and each front-end had its own copy of the
// composition.
func When(rfc3339 string) string {
	stamp := Stamp(rfc3339)
	if stamp == Unknown {
		return stamp // no moment, so no age either — inventing one is what Unknown exists to avoid
	}
	return stamp + " (" + Ago(rfc3339) + ")"
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
