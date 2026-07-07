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
	"hash/fnv"

	"github.com/charmbracelet/lipgloss"

	"github.com/flo-at/sindri/internal/hub"
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

// projectColors is a palette of distinct 256-colour codes. A project's scheme is a
// (primary, accent) pair drawn from it by hashing the repoTag — len² combinations,
// enough that the handful of repos in view rarely collide, and stable per repo
// across sessions (no persistence, just the hash).
var projectColors = []lipgloss.Color{
	lipgloss.Color("39"), lipgloss.Color("75"), lipgloss.Color("141"), lipgloss.Color("213"),
	lipgloss.Color("208"), lipgloss.Color("78"), lipgloss.Color("220"), lipgloss.Color("44"),
	lipgloss.Color("170"), lipgloss.Color("202"), lipgloss.Color("111"), lipgloss.Color("150"),
}

// projectScheme returns a project's deterministic (primary, accent) colours by tag.
func projectScheme(tag string) (primary, accent lipgloss.Color) {
	n := uint32(len(projectColors))
	h := fnv.New32a()
	h.Write([]byte(tag))
	sum := h.Sum32()
	return projectColors[sum%n], projectColors[(sum/n)%n]
}

// projectStyle colours a repo label by its project's primary colour, so the same
// repo always reads in the same colour across the board. Empty tag → plain.
func projectStyle(tag string) lipgloss.Style {
	if tag == "" {
		return lipgloss.NewStyle()
	}
	primary, _ := projectScheme(tag)
	return lipgloss.NewStyle().Foreground(primary)
}

// isCriticalPriority reports whether a priority code is the top (critical) band.
func isCriticalPriority(code string) bool { return hub.PriorityLabel(code) == "critical" }
