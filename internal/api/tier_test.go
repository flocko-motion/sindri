package api

import "testing"

// TestParseTierAcceptsExactlyTheThreeWords: the one table both front-ends and the hub parse
// against — a fourth word here is a fourth word everywhere, so it is checked shut.
func TestParseTierAcceptsExactlyTheThreeWords(t *testing.T) {
	for _, w := range TierWords {
		if tier, ok := ParseTier(w); !ok || tier != w {
			t.Errorf("ParseTier(%q) = (%q, %v), want (%q, true)", w, tier, ok, w)
		}
	}
	for _, bad := range []string{"", "expert", "P2", "critical", "junior "} {
		if _, ok := ParseTier(bad); ok {
			t.Errorf("ParseTier(%q) = ok, want a refusal", bad)
		}
	}
}

// TestTierOrDefaultIsMidNeverAnExtreme: unset must read as mid — not senior, which would put
// every unrated task in an existing backlog on the most expensive worker, and not junior, since
// unrated is not evidence a task is easy.
func TestTierOrDefaultIsMidNeverAnExtreme(t *testing.T) {
	if got := TierOrDefault(""); got != "mid" {
		t.Errorf("TierOrDefault(\"\") = %q, want mid", got)
	}
	for _, tier := range TierWords {
		if got := TierOrDefault(tier); got != tier {
			t.Errorf("TierOrDefault(%q) = %q, want it unchanged", tier, got)
		}
	}
}
