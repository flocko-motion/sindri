package api

import "testing"

func TestApprovalCountCountsOnlyPassVerdicts(t *testing.T) {
	revs := []Review{
		{Author: "fili", Verdict: "pass"},
		{Author: "kili", Verdict: "pass", Advisory: true},
		{Author: "fili", Verdict: "changes"},
		{Author: "", Verdict: ""},
	}
	if n := ApprovalCount(revs); n != 2 {
		t.Errorf("ApprovalCount = %d, want 2 (advisory badges count toward what a person sees)", n)
	}
}

func TestPRApprovable(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   bool
	}{
		{"open", true},
		{"approved", true},
		{"rejected", false},
		{"merging", false},
		{"merged", false},
		{"scrapped", false},
	} {
		if got := PRApprovable(PR{Status: tc.status}); got != tc.want {
			t.Errorf("PRApprovable(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestStatusLabel(t *testing.T) {
	if got, want := StatusLabel("open", 0), "open"; got != want {
		t.Errorf("StatusLabel(open, 0) = %q, want %q", got, want)
	}
	if got, want := StatusLabel("approved", 0), "approved"; got != want {
		t.Errorf("StatusLabel(approved, 0) = %q, want %q — no badge yet, no count to show", got, want)
	}
	if got, want := StatusLabel("approved", 1), "approved(1)"; got != want {
		t.Errorf("StatusLabel(approved, 1) = %q, want %q", got, want)
	}
	if got, want := StatusLabel("approved", 2), "approved(2)"; got != want {
		t.Errorf("StatusLabel(approved, 2) = %q, want %q", got, want)
	}
	if got, want := StatusLabel("rejected", 2), "rejected"; got != want {
		t.Errorf("StatusLabel(rejected, 2) = %q, want %q — the count only decorates approved", got, want)
	}
}
