package theme

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// marks is every marker the front-ends draw, named as the code names it.
var marks = map[string]string{
	"MarkAssigned":   MarkAssigned,
	"MarkDialIn":     MarkDialIn,
	"MarkWarning":    MarkWarning,
	"MarkRetired":    MarkRetired,
	"MarkMail":       MarkMail,
	"MarkClearArmed": MarkClearArmed,
	"MarkNeedsUser":  MarkNeedsUser,
	"MarkPRFinal":    MarkPRFinal,
	"MarkPRInterim":  MarkPRInterim,
}

// TestEveryMarkerIsOneCell is the property the icons were chosen for. Layout breaks when the width
// the program computes disagrees with the width the terminal draws, and a marker counted short
// pushes every column after it out of line. One cell each, measured — the Nerd Font icons because
// the Private Use Area has no East Asian Width to argue over, the plain ones because they are
// ordinary narrow characters.
func TestEveryMarkerIsOneCell(t *testing.T) {
	for name, g := range marks {
		if w := ansi.StringWidth(g); w != 1 {
			t.Errorf("%s (%q) counts as %d cells, want 1 — a double-width icon range (Material "+
				"Design) reintroduces the disagreement these replaced", name, g, w)
		}
	}
}

// TestEveryMarkerIsDistinct: the markers appear side by side in one column, so two that render the
// same are indistinguishable exactly where they are read together.
func TestEveryMarkerIsDistinct(t *testing.T) {
	seen := map[string]string{}
	for name, g := range marks {
		if prev, dup := seen[g]; dup {
			t.Errorf("%s and %s are the same glyph %q", prev, name, g)
		}
		seen[g] = name
	}
}

// TestTheIconsArePrivateUseArea pins where the pictorial markers come from. A patched font is what
// draws them; the guarantee that a row still lines up WITHOUT one comes from the PUA, so a marker
// that wandered out of it would take the guarantee with it.
func TestTheIconsArePrivateUseArea(t *testing.T) {
	for name, g := range map[string]string{
		"MarkAssigned": MarkAssigned, "MarkDialIn": MarkDialIn,
		"MarkWarning": MarkWarning, "MarkRetired": MarkRetired, "MarkMail": MarkMail,
	} {
		r := []rune(g)
		if len(r) != 1 {
			t.Errorf("%s is %d runes; a marker is one codepoint, with no variation selector to argue about", name, len(r))
			continue
		}
		if r[0] < 0xE000 || r[0] > 0xF8FF {
			t.Errorf("%s is U+%04X, outside the Private Use Area — that is where the width agreement comes from", name, r[0])
		}
	}
}
