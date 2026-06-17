// package: tui / component_field
// type:    ui components (generic, reusable form elements)
// job:     the building blocks a form is made of. Each field owns its own
//          editing behaviour and current value behind a small interface; the
//          form (component_form.go) only handles focus and layout. Text is
//          edited manually (not via bubbles/textinput) so a field's display
//          stays plain — paddable without ANSI miscounts.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// field is one editable element of a form.
type field interface {
	label() string
	value() string             // the value to submit
	display() string           // plain text shown in the row
	focus()                    // gain focus (form-driven)
	blur()                     // lose focus
	update(tea.KeyMsg) tea.Cmd // handle an edit key; nav keys are the form's
}

// --- text field ---

type textField struct {
	name string
	val  []rune
	foc  bool
}

func newTextField(name, val string) *textField { return &textField{name: name, val: []rune(val)} }

func (f *textField) label() string { return f.name }
func (f *textField) value() string { return string(f.val) }
func (f *textField) focus()        { f.foc = true }
func (f *textField) blur()         { f.foc = false }

func (f *textField) update(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyRunes:
		f.val = append(f.val, msg.Runes...)
	case tea.KeySpace:
		f.val = append(f.val, ' ')
	case tea.KeyBackspace:
		if n := len(f.val); n > 0 {
			f.val = f.val[:n-1]
		}
	}
	return nil
}

func (f *textField) display() string {
	if f.foc {
		return string(f.val) + "▏" // a plain caret marks the focused field
	}
	if len(f.val) == 0 {
		return "—"
	}
	return string(f.val)
}

// --- choice field ---

type choiceField struct {
	name    string
	options []string // display labels
	values  []string // parallel codes returned by value()
	cur     int
}

func newChoiceField(name string, options, values []string, sel string) *choiceField {
	f := &choiceField{name: name, options: options, values: values}
	for i, v := range values {
		if v == sel {
			f.cur = i
		}
	}
	return f
}

func (f *choiceField) label() string { return f.name }
func (f *choiceField) value() string { return f.values[f.cur] }
func (f *choiceField) focus()        {}
func (f *choiceField) blur()         {}

func (f *choiceField) update(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "left", "h":
		f.cur = (f.cur - 1 + len(f.options)) % len(f.options)
	case "right", "l", " ":
		f.cur = (f.cur + 1) % len(f.options)
	}
	return nil
}

func (f *choiceField) display() string { return "‹ " + f.options[f.cur] + " ›" }
