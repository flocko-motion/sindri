// package: tui / viewport
// type:    ui (viewport sync)
// job:     keep the active tab's cursor and its list/detail viewports in range as
// the model changes, and fetch the selected item's rich detail when the
// selection changes — the bridge between navigation and what's on screen.
// limits:  mutates viewport/cursor state and issues detail fetches; the geometry
// it consults is layout.go, the rows it clamps to are items.go.
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/ui/tui/scroll"
)

// reclamp keeps the active tab's cursor + both viewports in range.
func (m *model) reclamp() {
	n := len(m.rows())
	m.cursor[m.tab] = clampInt(m.cursor[m.tab], 0, max(0, n-1))
	listH := m.bodyHeight()
	switch m.tab { // agents/prs: the list is the short top region of a split (any width)
	case 1:
		listH = m.agentListHeight()
	case 2:
		listH = m.prListHeight()
	}
	m.list.SetHeight(listH)
	m.list.SetTotal(n)
	m.list.SetCursor(m.cursor[m.tab])
	// Offset-driven scroll (J/K), preserved across re-layouts; reset to top only
	// when the selection changes (syncDetail).
	if m.tab == 2 { // PRs: detail pane is the big bottom-left content (any width)
		m.detail.Resize(max(1, m.bodyHeight()-m.prListHeight()-1), len(m.prContentLines()))
		// The right column is a second scrollable region on this tab. Resize, not SetCursor: it is
		// offset-driven like the diff pane, and re-following a cursor here would drag the view back
		// on every poll — the way a scroll that does not survive a tick reads as one that never
		// happened.
		lines, _ := m.prMetaLines(max(1, m.w-m.prContentWidth()-1))
		m.prMeta.Resize(m.bodyHeight(), len(lines))
	} else if m.tab == 0 || m.tab == 3 { // generic detail pane: size to the WRAPPED count
		wrapped, _ := wrapContentMapped(m.detailLines(), m.detailWidth())
		m.detail.Resize(m.bodyHeight(), len(wrapped))
	} else if m.tab == 1 { // Agents: right column wraps like PRs' meta column (agentsBody)
		m.detail.Resize(m.bodyHeight(), len(wrapMeta(m.agentItems(), m.agentDetailWidth())))
	} else {
		m.detail.Resize(m.bodyHeight(), len(m.detailLines()))
	}
}

// scrollTarget is the viewport J/K move. Every tab has one detail pane and J/K drive it from
// either side, which is the pinned rule. The PRs tab is the exception the rule did not foresee: it
// has TWO scrollable regions — the diff and the metadata column — and one pair of keys, so there
// they follow the focus. Without this the column's reviews, findings and history had no key at all
// and everything past the first screenful was unreachable.
func (m *model) scrollTarget() *scroll.Viewport {
	if m.tab == 2 && m.rightFocus {
		return &m.prMeta
	}
	return &m.detail
}

// syncDetail fetches the selected item's rich detail when the selection changes.
func (m *model) syncDetail() tea.Cmd {
	key := fmt.Sprintf("%d:%s", m.tab, m.selID())
	if key == m.detailKey || m.cl == nil {
		return nil
	}
	m.detailKey = key
	m.detail.ScrollTop() // new selection → show its detail from the top
	m.rightCursor = 0    // and reset the right-column cursor to its first item
	id := m.selID()
	if id == "" {
		return nil
	}
	cl := m.cl
	switch m.tab {
	case 0:
		return func() tea.Msg { t, _ := cl.TaskInfo(id); return taskMsg{id, t} }
	case 1:
		m.agentPane, m.agentPod, m.agentDiag, m.agentClients = "", "", "", nil // selection changed — drop the previous agent's screen/pod/clients
		m.agentView = "screen"                                                 // default back to the live screen
		return tea.Batch(
			func() tea.Msg { evs, _ := cl.Log(id); return logMsg{id, evs} },
			paneFetchCmd(cl, id),
			clientsFetchCmd(cl, id),
		)
	case 5:
		return func() tea.Msg { d, _ := cl.RunInfo(id); return runMsg{id, d} }
	default:
		m.prView = "diff" // new PR → show its diff (its stored lint loads via PRInfo)
		return func() tea.Msg { d, _ := cl.PRInfo(id); return prMsg{id, d} }
	}
}
