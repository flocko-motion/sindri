package harness

import (
	"testing"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/agent/claude"
)

// TestTierIsReadsAModelsFamilyNotItsExactID is why the join has one home. A tier dispatches to a
// plain id and a session answers under a more specific one — Haiku 4.5 runs as a dated snapshot —
// so the comparison is the backend's own, and a caller that reached for a bare equality got a
// mismatch that was never there.
//
// One did: the assignment tiebreak compared with ==, so a junior task never read as "the model it
// is already on" and every junior tie broke toward a switch nothing needed.
func TestTierIsReadsAModelsFamilyNotItsExactID(t *testing.T) {
	agentport.Use(claude.New())
	t.Cleanup(func() { agentport.Use(unreadablePane{}) })
	s, _ := newService(t)

	for _, c := range []struct {
		on, tier string
		met      bool
		why      string
	}{
		{"claude-haiku-4-5-20251001", "junior", true, "the dated snapshot IS the tier's model"},
		{"claude-haiku-4-5", "junior", true, "so is the plain id it dispatches to"},
		{"claude-sonnet-5", "mid", true, "an exact id still matches itself"},
		{"claude-opus-5", "senior", true, "as does the top tier's"},
		{"claude-opus-5", "junior", false, "a different family is a real mismatch"},
		{"claude-haiku-4-5-20251001", "senior", false, "and a snapshot of one is not another"},
	} {
		met, known := s.TierIs(c.on, c.tier)
		if !known {
			t.Errorf("TierIs(%q, %q): tier unknown, want it recognised", c.on, c.tier)
			continue
		}
		if met != c.met {
			t.Errorf("TierIs(%q, %q) = %v, want %v — %s", c.on, c.tier, met, c.met, c.why)
		}
	}
}

// TestTierIsRefusesATierItDoesNotKnow: known=false is the only honest answer for a rating the
// backend has no model for, and nothing may act on met beside it. A false read as "already on the
// right model" would hand the work over on whatever the session held, silently.
func TestTierIsRefusesATierItDoesNotKnow(t *testing.T) {
	agentport.Use(claude.New())
	t.Cleanup(func() { agentport.Use(unreadablePane{}) })
	s, _ := newService(t)

	if _, known := s.TierIs("claude-opus-5", "archmage"); known {
		t.Error("TierIs claimed to know the tier \"archmage\"")
	}
}
