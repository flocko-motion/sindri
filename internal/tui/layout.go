// package: tui / layout
// type:    ui (geometry)
// job:     the dashboard's size arithmetic — body height under the tab strip and
//          footer, and the master/detail column split — shared by every tab's
//          render so the panes line up.
// limits:  pure geometry off the model's width/height; no state, no rendering.
package tui

func (m model) bodyHeight() int {
	if h := m.h - 3; h > 0 { // tab strip (1) + footer (2)
		return h
	}
	return 1
}

func (m model) leftWidth() int {
	// The Tasks table (gutter + id + type + prio + state + title) needs room, so
	// give the selector ~60% — clamped so neither pane gets too narrow.
	w := m.w * 3 / 5
	return clampInt(w, 28, max(28, m.w-28))
}

func (m model) detailWidth() int {
	w := m.w - m.leftWidth() - 1 // 1 for the divider
	if w < 1 {
		w = 1
	}
	return w
}
