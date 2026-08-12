package theme

import "testing"

// TestPriorityNoneRename: the lowest tier now reads "none" everywhere, but the old
// input words still resolve to P4 so muscle memory and stored input keep working.
func TestPriorityNoneRename(t *testing.T) {
	if got := PriorityLabel("P4"); got != "none" {
		t.Errorf("PriorityLabel(P4) = %q, want none", got)
	}
	for _, w := range []string{"none", "trivial", "minor"} {
		if got := PriorityCode(w); got != "P4" {
			t.Errorf("PriorityCode(%q) = %q, want P4", w, got)
		}
	}
	last := PriorityWords[len(PriorityWords)-1]
	if last != "none" {
		t.Errorf("PriorityWords should end in none, got %q", last)
	}
	for _, w := range PriorityWords {
		if w == "trivial" {
			t.Error("PriorityWords must not advertise the old word trivial")
		}
	}
}
