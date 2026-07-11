// package: tui / screenshot
// type:    dev/test harness
// job:     render the dashboard headlessly (no terminal, no hub) so layout can
//          be eyeballed and asserted from tests, replaying keypresses through
//          the real update path so modals and forms are driven exactly as live.
// limits:  a test/dev harness only; it is not part of the live app wiring
//          (-> tui.go).
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/hub"
)

// Screenshot renders the dashboard at w×h with the given board state and a
// sequence of key presses applied in order. Returned cmds are not run (no hub
// calls), so detail panes show their synchronous content only.
//
//deadcode:keep — dev/test harness for headless rendering
func Screenshot(st hub.BoardState, w, h int, keys ...string) string {
	var tm tea.Model = newModel(nil, nil, "")
	m := tm.(model)
	m.w, m.h = w, h
	m.state = st
	m.reclamp()
	tm = m
	for _, k := range keys {
		tm, _ = tm.Update(keyMsg(k)) // the real update path, so modals are driven
	}
	return tm.View()
}

// keyMsg maps a key name (as used by tea.KeyMsg.String) back to a KeyMsg, so
// the harness can replay navigation through the full update path.
//
//deadcode:keep — used by Screenshot
func keyMsg(k string) tea.KeyMsg {
	named := map[string]tea.KeyType{
		"tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab, "enter": tea.KeyEnter,
		"esc": tea.KeyEsc, "up": tea.KeyUp, "down": tea.KeyDown,
		"left": tea.KeyLeft, "right": tea.KeyRight, "backspace": tea.KeyBackspace,
		" ": tea.KeySpace, "ctrl+s": tea.KeyCtrlS, "ctrl+d": tea.KeyCtrlD, "ctrl+u": tea.KeyCtrlU,
		"ctrl+l": tea.KeyCtrlL, "ctrl+h": tea.KeyCtrlH,
	}
	if k == " " {
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	}
	if t, ok := named[k]; ok {
		return tea.KeyMsg{Type: t}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}
