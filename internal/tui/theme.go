// package: tui / theme
// type:    ui (global colour scheme)
// job:     the one place colours live. Status drives row colour: tasks are pink
//          when active, green when open, grey when done; agents are grey when
//          down, yellow while transitioning, green when running. Critical
//          priority is red. Everything else renders in the terminal's default.
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
	cYellow = lipgloss.Color("220") // transitioning (launching/stopping)
)

var (
	stActive = lipgloss.NewStyle().Foreground(cPink)
	stOpen   = lipgloss.NewStyle().Foreground(cGreen)
	stDone   = lipgloss.NewStyle().Foreground(cGrey)
	stCrit   = lipgloss.NewStyle().Foreground(cRed)
	stWarn   = lipgloss.NewStyle().Foreground(cYellow)
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

// agentStatusStyle is an agent row's colour: grey down, yellow transitioning,
// green running (idle/working/submitted).
func agentStatusStyle(status string) lipgloss.Style {
	switch status {
	case "down":
		return stDone
	case "launching", "stopping":
		return stWarn
	default:
		return stOpen
	}
}

// isCriticalPriority reports whether a priority code is the top (critical) band.
func isCriticalPriority(code string) bool { return hub.PriorityLabel(code) == "critical" }
