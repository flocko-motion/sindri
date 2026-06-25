// package: tui / component_field
// type:    ui components (generic, reusable form elements)
// job:     the building blocks a form is made of. Each field owns its own
//          editing, value, and rendering (a 1+ line block); the form
//          (component_form.go) handles only focus, layout, and the frame.
//          Text/choice are edited manually; the textarea wraps bubbles/textarea.
// limits:  a field edits and renders only itself; focus, layout, and the frame
//          are the form's (-> component_form.go).
package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const formLabelW = 12

// field is one editable element of a form.
type field interface {
	value() string
	focus()
	blur()
	grows() bool               // true ⇒ wants to fill the form's spare height
	resize(width, height int)  // layout hint from the form
	update(tea.KeyMsg) tea.Cmd // handle an edit key; nav keys are the form's
	render(active bool) string // the field's block (height lines), width-padded
}

// fieldLine lays out one "marker label value" row padded/highlighted to width.
func fieldLine(label, value string, width int, active bool) string {
	marker := "  "
	if active {
		marker = "▸ "
	}
	body := marker + padTrunc(label, formLabelW) + " " + value
	if active {
		return selStyle.Width(width).Render(body)
	}
	return lipgloss.NewStyle().Width(width).Render(body)
}

// --- text field (single line) ---

type textField struct {
	name string
	val  []rune
	foc  bool
	w    int
}

func newTextField(name, val string) *textField { return &textField{name: name, val: []rune(val)} }

func (f *textField) value() string   { return string(f.val) }
func (f *textField) focus()           { f.foc = true }
func (f *textField) blur()            { f.foc = false }
func (f *textField) grows() bool      { return false }
func (f *textField) resize(w, _ int)  { f.w = w }

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

func (f *textField) render(active bool) string {
	v := string(f.val)
	if active {
		v += "▏" // a plain caret marks the focused field
	} else if len(f.val) == 0 {
		v = "—"
	}
	return fieldLine(f.name, v, f.w, active)
}

// --- choice field (cycle one of a fixed set) ---

type choiceField struct {
	name    string
	options []string // display labels
	values  []string // parallel codes returned by value()
	cur     int
	w       int
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

func (f *choiceField) value() string   { return f.values[f.cur] }
func (f *choiceField) focus()          {}
func (f *choiceField) blur()           {}
func (f *choiceField) grows() bool     { return false }
func (f *choiceField) resize(w, _ int) { f.w = w }

func (f *choiceField) update(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "left", "h":
		f.cur = (f.cur - 1 + len(f.options)) % len(f.options)
	case "right", "l", " ":
		f.cur = (f.cur + 1) % len(f.options)
	}
	return nil
}

func (f *choiceField) render(active bool) string {
	return fieldLine(f.name, "‹ "+f.options[f.cur]+" ›", f.w, active)
}

// --- textarea field (multi-line) ---

type textareaField struct {
	name string
	ta   textarea.Model
	w    int
}

func newTextareaField(name, val string) *textareaField {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = "  "
	ta.SetValue(val)
	ta.MaxHeight = 0 // grow with the assigned height
	return &textareaField{name: name, ta: ta}
}

func (f *textareaField) value() string { return f.ta.Value() }
func (f *textareaField) focus()        { f.ta.Focus() }
func (f *textareaField) blur()         { f.ta.Blur() }
func (f *textareaField) grows() bool   { return true }

func (f *textareaField) resize(w, h int) {
	f.w = w
	f.ta.SetWidth(w)
	if h < 2 {
		h = 2
	}
	f.ta.SetHeight(h - 1) // reserve one line for the label
}

func (f *textareaField) update(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	f.ta, cmd = f.ta.Update(msg)
	return cmd
}

func (f *textareaField) render(active bool) string {
	return fieldLine(f.name, "", f.w, active) + "\n" + f.ta.View()
}
