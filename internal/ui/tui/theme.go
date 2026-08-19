// package: tui / theme
// type:    ui (global colour scheme)
// job:     the one place colours live. Every tab colours a row by WHAT THE USER SHOULD
// DO: red stopped and only they can unstop it, cyan work in flight, green
// proceeding, yellow their move but not stopped, orange transitioning, grey
// finished. Critical priority is pink, in its own column.
// limits:  colours only; no layout or data logic (-> the component/tab that
// uses them).
package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// The palette. 256-colour codes so it works on basic terminals.
var (
	cPink   = lipgloss.Color("211") // critical priority, in its own column
	cGreen  = lipgloss.Color("78")  // open / running
	cGrey   = lipgloss.Color("244") // done / down
	cRed    = lipgloss.Color("203") // critical
	cYellow = lipgloss.Color("220") // idle / orphan
	cOrange = lipgloss.Color("208") // transitioning (launching/stopping)
	cCyan   = lipgloss.Color("80")  // work in flight — the healthiest state, so not the loudest
)

var (
	// stPrio marks the critical priority band. Pink rather than red since red was given one
	// meaning across every tab (below), and a critical task being worked is not stopped at all —
	// it would have been the only red thing on the board saying "nothing to do here".
	stPrio = lipgloss.NewStyle().Foreground(cPink)
	stOpen = lipgloss.NewStyle().Foreground(cGreen)
	stDone = lipgloss.NewStyle().Foreground(cGrey)
	// stCrit is RED, and red means one thing across every tab: stopped, and only the user can
	// unstop it. It is the (N!) attention badge rendered a second way — the badge counts the rows
	// waiting on the user, red says THIS row is one of them — so both must come from one predicate
	// (api.AgentNeedsUser, api.PRNeedsUser, api.AwaitingVerdict). A colour that re-derived the
	// condition would drift from the badge the first time a state was added, and neither would
	// look wrong on its own.
	stCrit  = lipgloss.NewStyle().Foreground(cRed)
	stWarn  = lipgloss.NewStyle().Foreground(cYellow)
	stTrans = lipgloss.NewStyle().Foreground(cOrange)
	// stWorking is work in flight and nobody's move: an agent building, a rejected PR being
	// reworked. Positive rather than alarming, because it is the healthiest state on the board.
	stWorking = lipgloss.NewStyle().Foreground(cCyan)
)

// Diff colours, with a FORCED light foreground so they survive any terminal theme. Headers are
// tinted, not backgrounded, so they read as structure rather than content.
var (
	diffAddStyle  = lipgloss.NewStyle().Background(lipgloss.Color("22")).Foreground(lipgloss.Color("231"))
	diffDelStyle  = lipgloss.NewStyle().Background(lipgloss.Color("52")).Foreground(lipgloss.Color("231"))
	diffHunkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("44")).Bold(true)
	diffMetaStyle = lipgloss.NewStyle().Foreground(cGrey).Bold(true)
)

// taskStatusStyle colours a task by its status alone: cyan while it is being worked, grey when
// done, green otherwise. A task stopped behind a gate is redder than any of these, and that is the
// caller's to apply (-> taskRows), since it reads the whole tree to know.
func taskStatusStyle(status string) lipgloss.Style {
	if status == "in_progress" {
		return stWorking
	}
	if api.DoneStatus(status) {
		return stDone
	}
	return stOpen
}

// agentStatusStyle colours by what you should DO: red blocked (needs you now), yellow idle (your
// move), green working (leave it), orange transitioning, grey down.
func agentStatusStyle(status string) lipgloss.Style {
	switch status {
	case "down", "stopped":
		return stDone
	case "launching", "stopping":
		return stTrans
	case "blocked", "stalled", "full", "signed-out", "api-error", "launch-failed":
		return stCrit // all need a human; a signed-out agent cannot even be told anything
	case "idle":
		return stWarn
	default:
		return stOpen
	}
}

// prStatusStyle colours a PR by what you should DO. Red is not decided here: it is exactly
// api.PRNeedsUser, the predicate the PRs badge counts, so a row cannot be red and uncounted or
// counted and not red. Everything else follows the same cross-tab meanings: grey finished, orange
// mid-merge, cyan for the worker's rework, green for a review that is coming.
func prStatusStyle(p api.PR, agents []api.AgentView, merging bool) lipgloss.Style {
	if merging { // the user's merge is in flight — transitioning, as an agent launching is
		return stTrans
	}
	if api.PRNeedsUser(p, agents) {
		return stCrit
	}
	switch p.Status {
	case "merged", "scrapped":
		return stDone
	case "merging":
		return stTrans
	case "rejected":
		return stWorking // the WORKER's move: rework in flight, nothing for the user to act on
	}
	return stOpen // open with a reviewer alive in its repo: proceeding, leave it
}

// repoStyleFor colours text in a repo's bright shade; an empty tag stays plain. The palette and
// the index→colour mapping live in ui/theme, shared with the CLI.
func repoStyleFor(tag string, choice int) lipgloss.Style {
	if tag == "" {
		return lipgloss.NewStyle()
	}
	_, bright := theme.RepoColors(tag, choice)
	return lipgloss.NewStyle().Foreground(bright)
}

// isCriticalPriority reports whether a priority code is the top (critical) band.
func isCriticalPriority(code string) bool { return theme.PriorityLabel(code) == "critical" }
