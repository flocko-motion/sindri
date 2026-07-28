// package: tui / theme
// type:    ui (global colour scheme)
// job:     the one place colours live. Status drives row colour: tasks are pink
//          when active, green when open, grey when done; agents are grey when
//          down, yellow while transitioning, green when running. Critical
//          priority is red. Everything else renders in the terminal's default.
// limits:  colours only; no layout or data logic (-> the component/tab that
//          uses them).
package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// The palette. 256-colour codes so it works on basic terminals.
var (
	cPink   = lipgloss.Color("211") // active / in-progress
	cGreen  = lipgloss.Color("78")  // open / running
	cGrey   = lipgloss.Color("244") // done / down
	cRed    = lipgloss.Color("203") // critical
	cYellow = lipgloss.Color("220") // idle / orphan
	cOrange = lipgloss.Color("208") // transitioning (launching/stopping)
)

var (
	stActive = lipgloss.NewStyle().Foreground(cPink)
	stOpen   = lipgloss.NewStyle().Foreground(cGreen)
	stDone   = lipgloss.NewStyle().Foreground(cGrey)
	stCrit   = lipgloss.NewStyle().Foreground(cRed)
	stWarn   = lipgloss.NewStyle().Foreground(cYellow)
	stTrans  = lipgloss.NewStyle().Foreground(cOrange)
)

// Diff colours — the classic editor look: additions on a dark green background,
// removals on dark red, both with a forced light foreground so they stay legible
// regardless of the terminal's own theme. Hunk and file headers are tinted, not
// backgrounded, so they read as structure rather than content.
var (
	diffAddStyle  = lipgloss.NewStyle().Background(lipgloss.Color("22")).Foreground(lipgloss.Color("231"))
	diffDelStyle  = lipgloss.NewStyle().Background(lipgloss.Color("52")).Foreground(lipgloss.Color("231"))
	diffHunkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("44")).Bold(true)
	diffMetaStyle = lipgloss.NewStyle().Foreground(cGrey).Bold(true)
)

// taskStatusStyle is a task row's colour: pink active, grey done, green otherwise
// (open, in_review, …).
func taskStatusStyle(status string) lipgloss.Style {
	switch status {
	case "in_progress":
		return stActive
	case "closed", "approved", "merged":
		return stDone
	default:
		return stOpen
	}
}

// agentStatusStyle is an agent row's colour, tuned to what you should DO: grey down,
// orange transitioning (launching/stopping), red blocked (it needs your attention
// now), yellow idle (not doing anything — your move), green working (leave it) and
// the workflow phases (submitted/collab/…).
func agentStatusStyle(status string) lipgloss.Style {
	switch status {
	case "down":
		return stDone
	case "launching", "stopping":
		return stTrans
	case "blocked":
		return stCrit
	case "idle":
		return stWarn
	default:
		return stOpen
	}
}

// A project's colour is one hue (deterministic from its repoTag) rendered in two
// shades: a bright shade for text/labels on the terminal's dark background, and a
// muted dark shade for a filled background (the header bar). Same hue → the same
// repo always reads the same; two shades → a guaranteed dark/bright contrast pair,
// none at full intensity. Derived in truecolour (HSL) so lightness is controllable.
const (
	repoDarkSat,   repoDarkLight   = 0.32, 0.22 // muted, dark: for filled backgrounds
	repoBrightSat, repoBrightLight = 0.55, 0.72 // bright: for text on a dark background
)

// nRepoColors is the size of the pickable colour palette: evenly-spaced hues around
// the wheel, so a repo can be pinned to a chosen colour (1..nRepoColors) instead of
// the hash-derived default (0).
const nRepoColors = 24

// paletteHue is the hue (degrees) for a 1-based palette choice.
func paletteHue(choice int) float64 { return float64(((choice - 1) * 360 / nRepoColors) % 360) }

// projectHue maps a repoTag to a stable hue in [0,360) — the default when no colour
// is pinned. The derivation lives in ui/theme so a name gets the same hue here, in the
// CLI, and for chat participants.
func projectHue(tag string) float64 { return theme.Hue(tag) }

// hueFor is a repo's hue: a pinned palette choice (1..nRepoColors) if set, else the
// hash-derived default.
func hueFor(tag string, choice int) float64 {
	if choice >= 1 && choice <= nRepoColors {
		return paletteHue(choice)
	}
	return projectHue(tag)
}

// repoColorsFor returns a repo's (dark, bright) shades for a colour choice — the same
// hue at two lightnesses, a ready contrast pair for a filled bar (dark bg + bright fg).
func repoColorsFor(tag string, choice int) (dark, bright lipgloss.Color) {
	hue := hueFor(tag, choice)
	return lipgloss.Color(hslHex(hue, repoDarkSat, repoDarkLight)),
		lipgloss.Color(hslHex(hue, repoBrightSat, repoBrightLight))
}

// repoStyleFor colours text in a repo's bright shade for a colour choice. Empty tag
// → plain.
func repoStyleFor(tag string, choice int) lipgloss.Style {
	if tag == "" {
		return lipgloss.NewStyle()
	}
	_, bright := repoColorsFor(tag, choice)
	return lipgloss.NewStyle().Foreground(bright)
}

// hslHex converts an HSL colour (h in [0,360), s,l in [0,1]) to a "#rrggbb" string.
func hslHex(h, s, l float64) string { return theme.HSLHex(h, s, l) }

// isCriticalPriority reports whether a priority code is the top (critical) band.
func isCriticalPriority(code string) bool { return hub.PriorityLabel(code) == "critical" }
