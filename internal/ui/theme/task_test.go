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

// TestApprovalLabelNamesTheMissingAction: "pending" names a state, "unapproved" names what is
// missing — which is the question a reader has when a task is sitting still. The stored value keeps
// its own spelling: every predicate branches on it, so only the display moves.
func TestApprovalLabelNamesTheMissingAction(t *testing.T) {
	if got := ApprovalLabel("pending"); got != "unapproved" {
		t.Errorf("ApprovalLabel(pending) = %q, want unapproved", got)
	}
	for _, s := range []string{"", "approved", "rejected"} {
		if got := ApprovalLabel(s); got != s {
			t.Errorf("ApprovalLabel(%q) = %q — the others already say what they are", s, got)
		}
	}
}
