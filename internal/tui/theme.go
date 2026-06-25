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

// agentStatusStyle is an agent row's colour: grey down, orange transitioning
// (launching/stopping), yellow idle, green working (working/submitted).
func agentStatusStyle(status string) lipgloss.Style {
	switch status {
	case "down":
		return stDone
	case "launching", "stopping":
		return stTrans
	case "idle":
		return stWarn
	default:
		return stOpen
	}
}

// isCriticalPriority reports whether a priority code is the top (critical) band.
func isCriticalPriority(code string) bool { return hub.PriorityLabel(code) == "critical" }
