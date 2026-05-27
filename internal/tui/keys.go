package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Quit        key.Binding
	Tab         key.Binding
	ShiftTab    key.Binding
	Up          key.Binding
	Down        key.Binding
	Enter       key.Binding
	Refresh     key.Binding
	PanelSwitch key.Binding
	DetailUp    key.Binding
	DetailDown  key.Binding
}

var keys = keyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab", "l"),
		key.WithHelp("tab", "next column"),
	),
	ShiftTab: key.NewBinding(
		key.WithKeys("shift+tab", "h"),
		key.WithHelp("shift+tab", "prev column"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	PanelSwitch: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "switch panel"),
	),
	DetailUp: key.NewBinding(
		key.WithKeys("K", "shift+up"),
		key.WithHelp("Shift+K", "scroll detail up"),
	),
	DetailDown: key.NewBinding(
		key.WithKeys("J", "shift+down"),
		key.WithHelp("Shift+J", "scroll detail down"),
	),
}
