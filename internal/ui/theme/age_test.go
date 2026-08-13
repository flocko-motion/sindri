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

// TestUnknownAgeSaysSo: a source that never gave a time must not render as "now" — a task from
// before this column existed would read as though it had just been filed — nor as a blank cell,
// which reads as a rendering gap rather than as an answer.
func TestUnknownAgeSaysSo(t *testing.T) {
	// The last is a zero time.Time formatted: valid RFC3339, year 1, and it would otherwise render
	// as ~105000 weeks rather than as the absence of an answer.
	for _, s := range []string{"", "not a date", "0001-01-01T00:00:00Z"} {
		if got := Age(s); got != Unknown {
			t.Errorf("Age(%q) = %q, want %q", s, got, Unknown)
		}
		if got := Stamp(s); got != Unknown {
			t.Errorf("Stamp(%q) = %q, want %q", s, got, Unknown)
		}
	}
	// It still has to fit the column it shares with real ages.
	if len(Unknown) > 4 {
		t.Errorf("%q is wider than the age column allows", Unknown)
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

// TestWhenPairsTheMomentWithItsAge: a detail pane shows created and changed side by side, so the
// two must be written the same way — one function, not a composition each front-end repeats.
func TestWhenPairsTheMomentWithItsAge(t *testing.T) {
	got := When(time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339))
	if !strings.Contains(got, "(2h ago)") {
		t.Errorf("When = %q, want the exact moment with its age beside it", got)
	}
	if !strings.HasPrefix(got, Stamp(time.Now().Add(-2*time.Hour).UTC().Format(time.RFC3339))) {
		t.Errorf("When = %q, want it to lead with the same stamp Stamp gives", got)
	}
}

// TestWhenSaysUnknownRatherThanInventingOne is the honest blank the mirrored sources need: a task
// whose source gives no timestamp is never "recent", and the detail saying so is what explains its
// absence from the active filter. A fallback to the created time would hide exactly that.
func TestWhenSaysUnknownRatherThanInventingOne(t *testing.T) {
	for _, s := range []string{"", "not a date"} {
		if got := When(s); got != Unknown {
			t.Errorf("When(%q) = %q, want %q with no age attached", s, got, Unknown)
		}
	}
}
