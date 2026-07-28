package theme

import (
	"strings"
	"testing"
)

// TestHueIsDeterministicAndSpread: the whole point of deriving a colour from the name is
// that nobody has to assign or store one — so the same name must map to the same hue
// every run, and different names must not collapse onto one hue.
func TestHueIsDeterministicAndSpread(t *testing.T) {
	if Hue("eitri") != Hue("eitri") {
		t.Error("Hue must be stable for the same name")
	}
	names := []string{"eitri", "dvalin", "brokk", "alviss", "hepti", "jari", "nabbi", "ivaldi"}
	seen := map[int]string{}
	for _, n := range names {
		h := int(Hue(n))
		if h < 0 || h >= 360 {
			t.Errorf("%s: hue %d out of [0,360)", n, h)
		}
		if other, dup := seen[h]; dup {
			t.Errorf("%s collides with %s at hue %d", n, other, h)
		}
		seen[h] = n
	}
}

// TestUserColourIsPinned: the human's colour must not depend on the label, so agents and
// the eye find them in the same place every time. It also must not be mistakable for an
// agent's derived colour.
func TestUserColourIsPinned(t *testing.T) {
	if got, want := string(NameColor(SenderUser)), HSLHex(userHue, 0.85, 0.62); got != want {
		t.Errorf("user colour = %s, want the pinned %s", got, want)
	}
	if NameColor(SenderUser) == NameColor("eitri") {
		t.Error("the user must not share an agent's colour")
	}
}

// TestIconMarksTheHuman is the property agents depend on: the human is visibly not a peer.
func TestIconMarksTheHuman(t *testing.T) {
	if Icon(SenderUser) != UserIcon {
		t.Errorf("the user needs the human icon, got %q", Icon(SenderUser))
	}
	if Icon(SenderSystem) != SystemIcon {
		t.Errorf("the hub's own lines need the system icon, got %q", Icon(SenderSystem))
	}
	if Icon("eitri") != AgentIcon {
		t.Errorf("an agent needs the robot icon, got %q", Icon("eitri"))
	}
	if UserIcon == AgentIcon {
		t.Error("the human and an agent must not share a glyph")
	}
}

// TestHSLHexFormat: the derived colour has to be a hex lipgloss accepts.
func TestHSLHexFormat(t *testing.T) {
	for _, h := range []float64{0, 59, 90, 150, 210, 270, 330, 359} {
		got := HSLHex(h, BrightSat, BrightLight)
		if !strings.HasPrefix(got, "#") || len(got) != 7 {
			t.Errorf("hue %v produced %q, want #rrggbb", h, got)
		}
	}
}
