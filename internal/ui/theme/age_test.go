package theme

import (
	"strings"
	"testing"
	"time"
)

// TestAgeIsOneUnitWideEnoughForAColumn: the column is read at a glance, so one unit answers it. The
// width matters as much as the words — a cell that grows shears every row beside it.
func TestAgeIsOneUnitWideEnoughForAColumn(t *testing.T) {
	ago := func(d time.Duration) string {
		return Age(time.Now().Add(-d).UTC().Format(time.RFC3339))
	}
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{10 * time.Second, "now"},
		{10 * time.Minute, "10m"},
		{90 * time.Minute, "1h"},
		{26 * time.Hour, "1d"},
		{9 * 24 * time.Hour, "1w"},
	} {
		if got := ago(c.d); got != c.want {
			t.Errorf("Age(%v ago) = %q, want %q", c.d, got, c.want)
		}
		if len(ago(c.d)) > 4 {
			t.Errorf("Age(%v ago) = %q — wider than the column allows", c.d, ago(c.d))
		}
	}
}

// TestUnknownAgeIsBlankNotZero: a source that never gave a time must not render as "now" — a task
// carried over from before this column existed would read as though it had just been filed.
func TestUnknownAgeIsBlankNotZero(t *testing.T) {
	// The last is a zero time.Time formatted: valid RFC3339, year 1, and it would otherwise render
	// as ~105000 weeks rather than as the absence of an answer.
	for _, s := range []string{"", "not a date", "0001-01-01T00:00:00Z"} {
		if got := Age(s); got != "" {
			t.Errorf("Age(%q) = %q, want empty", s, got)
		}
		if got := Stamp(s); got != "" {
			t.Errorf("Stamp(%q) = %q, want empty", s, got)
		}
	}
}

// TestStampIsExactAndLocal: the detail pane exists to answer what the rounded column cannot, so it
// carries the date and the time, in the reader's own zone.
func TestStampIsExactAndLocal(t *testing.T) {
	when := time.Now().Add(-3 * time.Hour)
	got := Stamp(when.UTC().Format(time.RFC3339))
	if want := when.Local().Format("2006-01-02 15:04"); got != want {
		t.Errorf("Stamp = %q, want %q", got, want)
	}
	if !strings.Contains(got, ":") {
		t.Errorf("a detail stamp should carry the time of day, got %q", got)
	}
}
