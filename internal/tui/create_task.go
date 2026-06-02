// package: tui / create_task
// type:    ui
// job:     the "new task" modal — collects title/type/priority/desc/review and
//          creates the task via the td adapter.
// limits:  no domain rules; creation goes through adapter/td.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/adapter/spec"
	"github.com/flo-at/sindri/internal/adapter/td"
	"github.com/flo-at/sindri/internal/issue"
)

var (
	taskTypes  = []string{"task", "bug", "feature", "chore"}
	priorities = []string{"P0", "P1", "P2", "P3", "P4"}
)

const (
	fieldTitle = iota
	fieldType
	fieldPriority
	fieldReview
	fieldDesc
	fieldCount
)

type createTaskModel struct {
	titleInput    textinput.Model
	descInput     textinput.Model
	typeIdx       int
	prioIdx       int
	reviewChecked bool
	activeField   int
	err           error
	submitted     bool
	projectRoot   string
	specName      string // non-empty when invoked from a spec-only row; adds spec:<name> label
	editingID     string // set in edit mode; submit dispatches td.Update instead of td.Create
	origLabels    []string // labels carried by the task before editing — non-review ones are preserved on submit
}

type taskCreatedMsg struct {
	id  string
	err error
}

// taskUpdatedMsg is the edit-mode counterpart of taskCreatedMsg.
type taskUpdatedMsg struct {
	id  string
	err error
}

func newCreateTaskModel(projectRoot, specName string) createTaskModel {
	// Fits the 60-wide modal minus border/padding (4) and the "  Title: "/
	// "  Desc:  " labels (9). Without an explicit Width, textinput truncates
	// the placeholder to one character — the user saw a stray "T"/"D".
	const inputWidth = 45

	ti := textinput.New()
	ti.Placeholder = "Task title (required)"
	ti.Focus()
	ti.CharLimit = 200
	ti.Width = inputWidth
	if specName != "" {
		// Pre-fill the title from the spec's proposal H1 so the user starts
		// from real wording instead of an empty field; spec.Title falls
		// back to the slug when proposal.md is missing or has no heading.
		ti.SetValue(spec.Title(projectRoot, specName))
	}

	di := textinput.New()
	di.Placeholder = "Description (optional)"
	di.CharLimit = 500
	di.Width = inputWidth

	return createTaskModel{
		titleInput:    ti,
		descInput:     di,
		typeIdx:       0, // task
		prioIdx:       2, // P2
		reviewChecked: true,
		activeField:   fieldTitle,
		projectRoot:   projectRoot,
		specName:      specName,
	}
}

// newEditTaskModel reuses the create-task modal in edit mode: same layout
// and inputs, pre-populated from t, and submit dispatches td.Update instead
// of td.Create. Other labels on the task (spec link, approved gates, etc.)
// are preserved through origLabels so toggling Review never silently drops
// them.
func newEditTaskModel(projectRoot string, t issue.Task) createTaskModel {
	const inputWidth = 45

	ti := textinput.New()
	ti.Placeholder = "Task title (required)"
	ti.SetValue(t.Title)
	ti.Focus()
	ti.CharLimit = 200
	ti.Width = inputWidth

	di := textinput.New()
	di.Placeholder = "Description — leave empty to keep current"
	di.CharLimit = 500
	di.Width = inputWidth

	return createTaskModel{
		titleInput:    ti,
		descInput:     di,
		typeIdx:       indexOf(taskTypes, t.Type),
		prioIdx:       indexOfWithDefault(priorities, t.Priority, 2),
		reviewChecked: hasLabel(t.Labels, "require-review-code"),
		activeField:   fieldTitle,
		projectRoot:   projectRoot,
		editingID:     t.ID,
		origLabels:    append([]string{}, t.Labels...),
	}
}

func indexOf(items []string, v string) int {
	for i, s := range items {
		if s == v {
			return i
		}
	}
	return 0
}

func indexOfWithDefault(items []string, v string, def int) int {
	for i, s := range items {
		if s == v {
			return i
		}
	}
	return def
}

func hasLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}

func (m createTaskModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m createTaskModel) Update(msg tea.Msg) (createTaskModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Space toggles review checkbox only when review field is active
		if msg.String() == " " && m.activeField == fieldReview {
			m.reviewChecked = !m.reviewChecked
			return m, nil
		}
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.submitted = false
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			m.activeField = (m.activeField + 1) % fieldCount
			m.focusActive()
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab"))):
			m.activeField = (m.activeField + fieldCount - 1) % fieldCount
			m.focusActive()
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if m.activeField == fieldReview {
				m.reviewChecked = !m.reviewChecked
				return m, nil
			}
			if m.activeField == fieldType || m.activeField == fieldPriority {
				if m.activeField == fieldType {
					m.typeIdx = (m.typeIdx + 1) % len(taskTypes)
				} else {
					m.prioIdx = (m.prioIdx + 1) % len(priorities)
				}
				return m, nil
			}
			title := strings.TrimSpace(m.titleInput.Value())
			if title == "" {
				m.err = fmt.Errorf("title is required")
				return m, nil
			}
			// New tasks must clear the 15-char min so we don't get "fix" / "wip"
			// titles. Editing skips the rule — existing tasks may have been
			// created via the td CLI which doesn't enforce it.
			if m.editingID == "" && len(title) < 15 {
				m.err = fmt.Errorf("title too short (min 15 chars, got %d)", len(title))
				return m, nil
			}
			return m, m.submit()
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			if m.activeField == fieldType {
				m.typeIdx = (m.typeIdx + len(taskTypes) - 1) % len(taskTypes)
				return m, nil
			}
			if m.activeField == fieldPriority {
				m.prioIdx = (m.prioIdx + len(priorities) - 1) % len(priorities)
				return m, nil
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			if m.activeField == fieldType {
				m.typeIdx = (m.typeIdx + 1) % len(taskTypes)
				return m, nil
			}
			if m.activeField == fieldPriority {
				m.prioIdx = (m.prioIdx + 1) % len(priorities)
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	switch m.activeField {
	case fieldTitle:
		m.titleInput, cmd = m.titleInput.Update(msg)
	case fieldDesc:
		m.descInput, cmd = m.descInput.Update(msg)
	}
	return m, cmd
}

func (m *createTaskModel) focusActive() {
	m.titleInput.Blur()
	m.descInput.Blur()
	switch m.activeField {
	case fieldTitle:
		m.titleInput.Focus()
	case fieldDesc:
		m.descInput.Focus()
	}
}

func (m createTaskModel) submit() tea.Cmd {
	title := strings.TrimSpace(m.titleInput.Value())
	typ := taskTypes[m.typeIdx]
	prio := priorities[m.prioIdx]
	desc := strings.TrimSpace(m.descInput.Value())
	review := m.reviewChecked
	projectRoot := m.projectRoot
	specName := m.specName
	editingID := m.editingID
	origLabels := m.origLabels

	return func() tea.Msg {
		if editingID != "" {
			// Preserve every non-review label (spec:..., approved-*, etc.)
			// so toggling the review checkbox doesn't silently drop them.
			labels := make([]string, 0, len(origLabels)+1)
			for _, l := range origLabels {
				if l == "require-review-code" {
					continue
				}
				labels = append(labels, l)
			}
			if review {
				labels = append(labels, "require-review-code")
			}
			err := td.Update(projectRoot, editingID, td.UpdateOpts{
				Title: title, Type: typ, Priority: prio, Body: desc, Labels: labels,
			})
			return taskUpdatedMsg{id: editingID, err: err}
		}
		var labels []string
		if review {
			labels = append(labels, "require-review-code")
		}
		if specName != "" {
			labels = append(labels, "spec:"+specName)
		}
		out, err := td.Create(projectRoot, title, td.CreateOpts{Type: typ, Priority: prio, Body: desc, Labels: labels})
		if err != nil {
			return taskCreatedMsg{err: err}
		}
		return taskCreatedMsg{id: out}
	}
}

func (m createTaskModel) View(width, height int) string {
	modalW := 60
	if modalW > width-4 {
		modalW = width - 4
	}

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(highlight).
		Padding(1, 2).
		Width(modalW)

	var b strings.Builder
	heading := "New Task"
	if m.editingID != "" {
		heading = "Edit Task — " + m.editingID
	}
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(highlight).Render(heading))
	b.WriteString("\n\n")

	if m.specName != "" {
		b.WriteString(dimStyle.Render("  Linked to spec: "))
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("📄 " + m.specName))
		b.WriteString("\n\n")
	}

	// Title
	label := "  Title: "
	if m.activeField == fieldTitle {
		label = "> Title: "
	}
	b.WriteString(label)
	b.WriteString(m.titleInput.View())
	b.WriteString("\n\n")

	// Type
	label = "  Type:  "
	if m.activeField == fieldType {
		label = "> Type:  "
	}
	b.WriteString(label)
	b.WriteString(renderSelector(taskTypes, m.typeIdx, m.activeField == fieldType))
	b.WriteString("\n\n")

	// Priority
	label = "  Prio:  "
	if m.activeField == fieldPriority {
		label = "> Prio:  "
	}
	b.WriteString(label)
	b.WriteString(renderSelector(priorities, m.prioIdx, m.activeField == fieldPriority))
	b.WriteString("\n\n")

	// Review
	label = "  Review:"
	if m.activeField == fieldReview {
		label = "> Review:"
	}
	checkbox := "☐ code review"
	if m.reviewChecked {
		checkbox = "☑ code review"
	}
	if m.activeField == fieldReview {
		checkbox = selectedItemStyle.Render(checkbox)
	}
	b.WriteString(label + " " + checkbox)
	b.WriteString("\n\n")

	// Description
	label = "  Desc:  "
	if m.activeField == fieldDesc {
		label = "> Desc:  "
	}
	b.WriteString(label)
	b.WriteString(m.descInput.View())
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("  " + m.err.Error()))
		b.WriteString("\n\n")
	}

	b.WriteString(dimStyle.Render("  tab:next field  h/l:select  enter:create  esc:cancel"))

	modal := modalStyle.Render(b.String())

	// Center the modal
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func renderSelector(items []string, selected int, active bool) string {
	var parts []string
	for i, item := range items {
		if i == selected {
			if active {
				parts = append(parts, selectedItemStyle.Render("["+item+"]"))
			} else {
				parts = append(parts, lipgloss.NewStyle().Bold(true).Render("["+item+"]"))
			}
		} else {
			parts = append(parts, dimStyle.Render(" "+item+" "))
		}
	}
	return strings.Join(parts, " ")
}
