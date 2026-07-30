// package: ui/theme / theme
// type:    logic (shared presentation primitives)
// job:     the deterministic name→colour mapping and the participant markers both
// front-ends share, so one name is the same colour and glyph in the CLI and
// the TUI instead of each package inventing its own.
// limits:  colour + glyph derivation only — no layout, no I/O, no widgets.
package theme

import (
	"fmt"
	"hash/fnv"
	"math"

	"github.com/charmbracelet/lipgloss"

	"github.com/flo-at/sindri/internal/hub/chat"
)

// Saturation/lightness pairs for a derived hue: Dark* for filled backgrounds, Bright*
// for text on a dark terminal. Exported so callers pick a shade without re-tuning.
const (
	DarkSat, DarkLight     = 0.32, 0.22
	BrightSat, BrightLight = 0.55, 0.72
)

// The participant markers and reserved sender labels come from the chat core, which
// stamps the same glyphs into the lines it types into agents' sessions. Re-exported here
// so a front-end needs only this package for presentation, while there is still exactly
// one definition — and it lives with the domain, not with the styling.
const (
	UserIcon     = chat.UserIcon
	AgentIcon    = chat.AgentIcon
	SystemIcon   = chat.SystemIcon
	SenderUser   = chat.SenderUser
	SenderSystem = chat.SenderSystem
	HelpText     = chat.HelpText
)

// Icon marks who is speaking: the human, the hub itself, or an agent.
func Icon(sender string) string { return chat.Icon(sender) }

// HelpLine is the in-room interface description, dimmed for a banner or hint line.
func HelpLine() string { return Dim().Render(HelpText) }

// Hue maps any string to a stable hue in [0,360) — same name, same colour, every run
// and every front-end, with no palette to assign or store.
func Hue(s string) float64 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return float64(h.Sum32() % 360)
}

// userHue is fixed rather than derived: the human is the one participant whose colour
// must never depend on what they happen to be called, so agents (and the eye) can find
// them instantly. A warm amber, away from the cool end most names land in.
const userHue = 35.0

// NameColor is a participant's text colour. Agents get a hash-derived hue; the user is
// pinned (see userHue) and the hub's own lines stay grey.
func NameColor(sender string) lipgloss.Color {
	switch sender {
	case SenderUser:
		return lipgloss.Color(HSLHex(userHue, 0.85, 0.62))
	case SenderSystem:
		return lipgloss.Color("245")
	}
	return lipgloss.Color(HSLHex(Hue(sender), BrightSat, BrightLight))
}

// NameStyle renders a participant's name in its own colour, bold so the speaker reads
// as a heading above what they said.
func NameStyle(sender string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(NameColor(sender)).Bold(true)
}

// BodyStyle renders what a participant SAID in their colour too — the same hue as their
// name, unbolded so the name still reads as the heading. Colouring only the name leaves
// the words themselves an undifferentiated block, which is exactly what makes a long
// transcript hard to follow: the colour has to run through the text to attribute it at a
// glance, especially once a message spans several lines.
func BodyStyle(sender string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(NameColor(sender))
}

// Dim is the muted style for secondary text (timestamps, hints).
func Dim() lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color("245")) }

// HSLHex converts an HSL colour (h in [0,360), s,l in [0,1]) to a "#rrggbb" string.
func HSLHex(h, s, l float64) string {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return fmt.Sprintf("#%02x%02x%02x", int((r+m)*255), int((g+m)*255), int((b+m)*255))
}
