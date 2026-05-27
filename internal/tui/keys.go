package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Quit       key.Binding
	Up         key.Binding
	Down       key.Binding
	Enter      key.Binding
	Refresh    key.Binding
	DetailUp   key.Binding
	DetailDown key.Binding
	NavLeft    key.Binding
	NavRight   key.Binding
	NavUp      key.Binding
	NavDown    key.Binding
}

var keys = keyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	DetailUp: key.NewBinding(
		key.WithKeys("K"),
		key.WithHelp("K", "scroll detail up"),
	),
	DetailDown: key.NewBinding(
		key.WithKeys("J"),
		key.WithHelp("J", "scroll detail down"),
	),
	NavLeft: key.NewBinding(
		key.WithKeys("ctrl+h"),
		key.WithHelp("C-h", "col left"),
	),
	NavRight: key.NewBinding(
		key.WithKeys("ctrl+l"),
		key.WithHelp("C-l", "col right"),
	),
	NavUp: key.NewBinding(
		key.WithKeys("ctrl+k"),
		key.WithHelp("C-k", "panel up"),
	),
	NavDown: key.NewBinding(
		key.WithKeys("ctrl+j"),
		key.WithHelp("C-j", "panel down"),
	),
}
