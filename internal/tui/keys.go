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
	Backlog    key.Binding
	Workers    key.Binding
	NewTask    key.Binding
	Approve    key.Binding
	Merge      key.Binding
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
		key.WithHelp("C-h", "focus left"),
	),
	NavRight: key.NewBinding(
		key.WithKeys("ctrl+l"),
		key.WithHelp("C-l", "focus right"),
	),
	Backlog: key.NewBinding(
		key.WithKeys("b"),
		key.WithHelp("b", "backlog view"),
	),
	Workers: key.NewBinding(
		key.WithKeys("w"),
		key.WithHelp("w", "workers view"),
	),
	NewTask: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new task"),
	),
	Approve: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "approve PR"),
	),
	Merge: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "merge PR"),
	),
}
