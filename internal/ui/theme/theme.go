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

	"github.com/flo-at/sindri/internal/api"
)

// Sat/lightness pairs for a derived hue, exported so callers pick a shade without re-tuning.
const (
	DarkSat, DarkLight     = 0.32, 0.22
	BrightSat, BrightLight = 0.55, 0.72
)

// Re-exported from internal/api, which the hub also stamps these same glyphs from: a
// front-end needs only this package, and there is still exactly one definition.
const (
	UserIcon     = api.UserIcon
	AgentIcon    = api.AgentIcon
	SystemIcon   = api.SystemIcon
	SenderUser   = api.SenderUser
	SenderSystem = api.SenderSystem
	HelpText     = api.ChatHelpText
)

// Icon marks who is speaking: the human, the hub itself, or an agent.
func Icon(sender string) string { return api.ChatIcon(sender) }

// HelpLine is the in-room interface description, dimmed for a banner or hint line.
func HelpLine() string { return Dim().Render(HelpText) }

// Hue maps a string to a stable hue: same name, same colour everywhere, with no palette to store.
func Hue(s string) float64 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return float64(h.Sum32() % 360)
}

// userHue is FIXED, not derived: the human's colour must not depend on what they are called. A warm
// amber, away from the cool end most names land in.
const userHue = 35.0

// NameColor: agents get a hash-derived hue, the user is pinned (userHue), the hub stays grey.
func NameColor(sender string) lipgloss.Color {
	switch sender {
	case SenderUser:
		return lipgloss.Color(HSLHex(userHue, 0.85, 0.62))
	case SenderSystem:
		return lipgloss.Color("245")
	}
	return lipgloss.Color(HSLHex(Hue(sender), BrightSat, BrightLight))
}

// NameStyle bolds the name, so a speaker reads as a heading above what they said.
func NameStyle(sender string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(NameColor(sender)).Bold(true)
}

// BodyStyle colours the WORDS too, unbolded. Colouring only names leaves the text an
// undifferentiated block — the hue has to run through it to attribute a multi-line message.
func BodyStyle(sender string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(NameColor(sender))
}

// Dim is the muted style for secondary text (timestamps, hints).
func Dim() lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color("245")) }

// One hue per project in two shades, so a repo always reads the same AND the pair is guaranteed to
// contrast. HSL, because lightness has to be controllable.
const (
	RepoDarkSat, RepoDarkLight     = 0.32, 0.22 // muted, dark: for filled backgrounds
	RepoBrightSat, RepoBrightLight = 0.55, 0.72 // bright: for text on a dark background
)

// NRepoColors is the pickable palette: evenly-spaced hues, so a repo can be pinned instead of
// taking the hash-derived default (0).
const NRepoColors = 24

// PaletteHue is the hue (degrees) for a 1-based palette choice.
func PaletteHue(choice int) float64 { return float64(((choice - 1) * 360 / NRepoColors) % 360) }

// RepoHue prefers a pinned palette choice, else the hash-derived default (same Hue every front-end
// derives a name's colour from, so an unpinned repo's default agrees with everything else).
func RepoHue(tag string, choice int) float64 {
	if choice >= 1 && choice <= NRepoColors {
		return PaletteHue(choice)
	}
	return Hue(tag)
}

// RepoColors is the (dark, bright) pair for a filled bar: one hue at two lightnesses.
func RepoColors(tag string, choice int) (dark, bright lipgloss.Color) {
	hue := RepoHue(tag, choice)
	return lipgloss.Color(HSLHex(hue, RepoDarkSat, RepoDarkLight)),
		lipgloss.Color(HSLHex(hue, RepoBrightSat, RepoBrightLight))
}

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
