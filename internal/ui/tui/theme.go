// package: tui / theme
// type:    ui (global colour scheme)
// job:     the one place colours live. Status drives row colour: tasks are pink
// when active, green when open, grey when done; agents are grey when
// down, yellow while transitioning, green when running. Critical
// priority is red. Everything else renders in the terminal's default.
// limits:  colours only; no layout or data logic (-> the component/tab that
// uses them).
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

// Diff colours, with a FORCED light foreground so they survive any terminal theme. Headers are
// tinted, not backgrounded, so they read as structure rather than content.
var (
	diffAddStyle  = lipgloss.NewStyle().Background(lipgloss.Color("22")).Foreground(lipgloss.Color("231"))
	diffDelStyle  = lipgloss.NewStyle().Background(lipgloss.Color("52")).Foreground(lipgloss.Color("231"))
	diffHunkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("44")).Bold(true)
	diffMetaStyle = lipgloss.NewStyle().Foreground(cGrey).Bold(true)
)

// taskStatusStyle: pink active, grey done, green otherwise.
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

// agentStatusStyle colours by what you should DO: red blocked (needs you now), yellow idle (your
// move), green working (leave it), orange transitioning, grey down.
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

// One hue per project in two shades, so a repo always reads the same AND the pair is guaranteed to
// contrast. HSL, because lightness has to be controllable.
const (
	repoDarkSat, repoDarkLight     = 0.32, 0.22 // muted, dark: for filled backgrounds
	repoBrightSat, repoBrightLight = 0.55, 0.72 // bright: for text on a dark background
)

// nRepoColors is the pickable palette: evenly-spaced hues, so a repo can be pinned instead of
// taking the hash-derived default (0).
const nRepoColors = 24

// paletteHue is the hue (degrees) for a 1-based palette choice.
func paletteHue(choice int) float64 { return float64(((choice - 1) * 360 / nRepoColors) % 360) }

// projectHue is the default hue for a tag. Derived in ui/theme so a name gets the same hue here,
// in the CLI, and in chat.
func projectHue(tag string) float64 { return theme.Hue(tag) }

// hueFor prefers a pinned palette choice, else the hash-derived default.
func hueFor(tag string, choice int) float64 {
	if choice >= 1 && choice <= nRepoColors {
		return paletteHue(choice)
	}
	return projectHue(tag)
}

// repoColorsFor is the (dark, bright) pair for a filled bar: one hue at two lightnesses.
func repoColorsFor(tag string, choice int) (dark, bright lipgloss.Color) {
	hue := hueFor(tag, choice)
	return lipgloss.Color(hslHex(hue, repoDarkSat, repoDarkLight)),
		lipgloss.Color(hslHex(hue, repoBrightSat, repoBrightLight))
}

// repoStyleFor colours text in a repo's bright shade; an empty tag stays plain.
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
