// package: tui / component_form
// type:    ui component (generic, reusable)
// job:     a fill-in form rendered inside the generic almost-full-screen modal
//          (modalFrame) — a stack of fields (component_field.go) with one
//          focused at a time. tab/⇧tab move between fields; ←/→ cycle a choice;
//          ↑↓ and other keys edit the focused field; ctrl+s validates then
//          submits; esc cancels. The form owns layout + nav, not the chrome.
// limits:  layout + navigation only; fields own their editing
//          (-> component_field.go) and the frame is the modal's (-> component_modal.go).
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))

// formState is the active form. apply reads the field values (it closes over
// the field pointers) and returns the hub action to run on submit; validate (if
// set) returns a non-empty message to block submit and show an error.
type formState struct {
	active   bool
	title    string
	fields   []field
	cur      int
	apply    func() tea.Cmd
	validate func() string
	err      string
}

func (f *formState) open(title string, fields []field, validate func() string, apply func() tea.Cmd) {
	f.active, f.title, f.fields, f.cur = true, title, fields, 0
	f.validate, f.apply, f.err = validate, apply, ""
	for i, fl := range fields {
		if i == 0 {
			fl.focus()
		} else {
			fl.blur()
		}
	}
}

func (f *formState) move(d int) {
	f.fields[f.cur].blur()
	n := len(f.fields)
	f.cur = (f.cur + d + n) % n
	f.fields[f.cur].focus()
}

// update handles a key; returns a submit cmd (and deactivates) or nil.
func (f *formState) update(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		f.active = false
		return nil
	case "ctrl+s":
		if f.validate != nil {
			if e := f.validate(); e != "" {
				f.err = e
				return nil
			}
		}
		f.active = false
		return f.apply()
	case "tab":
		f.err = ""
		f.move(1)
		return nil
	case "shift+tab":
		f.err = ""
		f.move(-1)
		return nil
	}
	f.err = "" // editing clears a stale validation error
	return f.fields[f.cur].update(msg)
}

func (f formState) view(screenW, screenH int) string {
	cw := modalInnerWidth(screenW)
	innerH := modalContentHeight(screenH)

	// Height budget: single-line fields take one row each; the one growing
	// field (the textarea) fills whatever is left.
	grow, fixed := -1, 0
	for i, fl := range f.fields {
		if fl.grows() {
			grow = i
		} else {
			fixed++
		}
	}
	growH := innerH - fixed
	if growH < 3 {
		growH = 3
	}
	for i, fl := range f.fields {
		if i == grow {
			fl.resize(cw, growH)
		} else {
			fl.resize(cw, 1)
		}
	}

	var lines []string
	for i, fl := range f.fields {
		lines = append(lines, strings.Split(fl.render(i == f.cur), "\n")...)
	}
	for len(lines) < innerH { // pad to a stable height
		lines = append(lines, "")
	}
	lines = lines[:innerH]

	footer := dimStyle.Render(padTrunc("tab/⇧tab field · ←/→ choose · ctrl+s save · esc cancel", cw))
	if f.err != "" {
		footer = errStyle.Render(padTrunc("⚠ "+f.err, cw))
	}
	return modalFrame(f.title, strings.Join(lines, "\n"), footer, screenW, screenH)
}
