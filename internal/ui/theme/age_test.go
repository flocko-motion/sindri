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

// TestAgoReadsAsProseUnderAMinute is the bug itself: three call sites appended " ago" to Age, whose
// sub-minute answer is already a whole phrase, so a mail just read said "read now ago".
func TestAgoReadsAsProseUnderAMinute(t *testing.T) {
	for _, d := range []time.Duration{0, 10 * time.Second, 59 * time.Second, -5 * time.Second} {
		got := Ago(time.Now().Add(-d).UTC().Format(time.RFC3339))
		if got != "just now" {
			t.Errorf("Ago(%v ago) = %q, want %q", d, got, "just now")
		}
		if strings.Contains(got, "now ago") {
			t.Errorf("Ago(%v ago) = %q — the phrase this exists to prevent", d, got)
		}
	}
}

// TestAgoCarriesTheMagnitudeAboveAMinute: past the sub-minute case the column's own form reads
// correctly as prose, so Ago spells it exactly as Age does and adds the word.
func TestAgoCarriesTheMagnitudeAboveAMinute(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{10 * time.Minute, "10m ago"},
		{90 * time.Minute, "1h ago"},
		{26 * time.Hour, "1d ago"},
		{9 * 24 * time.Hour, "1w ago"},
	} {
		stamp := time.Now().Add(-c.d).UTC().Format(time.RFC3339)
		if got := Ago(stamp); got != c.want {
			t.Errorf("Ago(%v ago) = %q, want %q", c.d, got, c.want)
		}
		if got := Ago(stamp); got != Age(stamp)+" ago" {
			t.Errorf("Ago = %q but Age = %q — the two forms must not spell a magnitude differently", got, Age(stamp))
		}
	}
}

// TestAgoSaysUnknownWithNoWordHungOnIt: an absent timestamp has no age, and "n/a ago" would be a
// claim about a moment no source gave.
func TestAgoSaysUnknownWithNoWordHungOnIt(t *testing.T) {
	for _, s := range []string{"", "not a date", "0001-01-01T00:00:00Z"} {
		if got := Ago(s); got != Unknown {
			t.Errorf("Ago(%q) = %q, want %q", s, got, Unknown)
		}
	}
}

// TestHeldNeverSaysForNow guards the nastier sibling of "now ago": the PR detail suffixes its status
// with " for "+age, and "open for now" is real English meaning the opposite — for the time being.
func TestHeldNeverSaysForNow(t *testing.T) {
	for _, d := range []time.Duration{0, 30 * time.Second, -2 * time.Second} {
		got := Held(time.Now().Add(-d).UTC().Format(time.RFC3339))
		if got != "under a minute" {
			t.Errorf("Held(%v) = %q, want %q", d, got, "under a minute")
		}
		if "for "+got == "for now" {
			t.Errorf("Held(%v) composes to %q, which reads as the opposite", d, "for "+got)
		}
	}
	// Above a minute it is the column's own magnitude, so a detail and a cell cannot disagree.
	stamp := time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339)
	if got, want := Held(stamp), Age(stamp); got != want {
		t.Errorf("Held = %q, Age = %q — one magnitude, spelled once", got, want)
	}
	if got := Held("not a date"); got != Unknown {
		t.Errorf("Held(unparseable) = %q, want %q — the caller drops the suffix on that", got, Unknown)
	}
}

// TestWhenReadsAsProseWhenItIsFresh: When composes the same phrase, so it carried the bug too — a
// detail pane on something just changed read "(now ago)". The task named three call sites; this
// was the fourth, inside the helper itself.
func TestWhenReadsAsProseWhenItIsFresh(t *testing.T) {
	got := When(time.Now().Add(-3 * time.Second).UTC().Format(time.RFC3339))
	if !strings.HasSuffix(got, "(just now)") {
		t.Errorf("When = %q, want it to end with %q", got, "(just now)")
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
