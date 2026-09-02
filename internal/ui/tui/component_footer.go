// package: tui / component_footer
// type:    ui component (generic)
// job:     render the two-row footer pinned to the bottom — row one the global
// navigation, row two the active tab's context actions — each padded to
// the full width.
// limits:  pure rendering; the hint text is supplied by the caller (-> tui.go /
// the active tab).
package tui

// footer renders the two footer rows (global nav, context actions) at width. Both arrive already
// fitted to it (-> shedMiddle, shedTail); padTrunc's own cut is the backstop, and cuts mid-word.
func footer(global, context string, width int) string {
	return dimStyle.Render(padTrunc(global, width)) + "\n" + dimStyle.Render(padTrunc(context, width))
}
