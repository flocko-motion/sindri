// package: tui / component_form
// type:    ui component (generic, reusable)
// job:     a centered modal form — a stack of fields (component_field.go) with
//          one focused at a time. tab/↑↓ move between fields; ←/→ cycle a
//          choice; any other key edits the focused field; ctrl+s submits via
//          the apply callback; esc cancels. Reusable for any fill-in prompt
//          (new task, edit task, launch options, …).
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	formLabelW = 12
	formValueW = 42
)

// formState is the active form. apply reads the field values (it closes over
// the field pointers) and returns the hub action to run on submit.
type formState struct {
	active bool
	title  string
	fields []field
	cur    int
	apply  func() tea.Cmd
}

func (f *formState) open(title string, fields []field, apply func() tea.Cmd) {
	f.active, f.title, f.fields, f.cur, f.apply = true, title, fields, 0, apply
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
		f.active = false
		return f.apply()
	case "tab", "down":
		f.move(1)
		return nil
	case "shift+tab", "up":
		f.move(-1)
		return nil
	}
	return f.fields[f.cur].update(msg)
}

func (f formState) view(screenW, screenH int) string {
	hint := "tab/↑↓ field · ←/→ choose · ctrl+s save · esc cancel"
	cw := formLabelW + 3 + formValueW
	if w := lipgloss.Width(f.title); w > cw {
		cw = w
	}
	if w := lipgloss.Width(hint); w > cw {
		cw = w
	}
	blank := strings.Repeat(" ", cw)
	lines := []string{modalTitleStyle.Render(padTrunc(f.title, cw)), blank}
	for i, fl := range f.fields {
		marker := "  "
		if i == f.cur {
			marker = "▸ "
		}
		row := padTrunc(marker+padTrunc(fl.label(), formLabelW)+" "+fl.display(), cw)
		if i == f.cur {
			row = selStyle.Render(row)
		}
		lines = append(lines, row)
	}
	lines = append(lines, blank, dimStyle.Render(padTrunc(hint, cw)))
	box := modalBorderStyle.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(screenW, screenH, lipgloss.Center, lipgloss.Center, box)
}
