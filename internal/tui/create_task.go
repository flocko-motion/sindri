package tui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	taskTypes  = []string{"task", "bug", "feature", "chore"}
	priorities = []string{"P0", "P1", "P2", "P3", "P4"}
)

const (
	fieldTitle = iota
	fieldType
	fieldPriority
	fieldDesc
	fieldCount
)

type createTaskModel struct {
	titleInput textinput.Model
	descInput  textinput.Model
	typeIdx    int
	prioIdx    int
	activeField int
	err        error
	submitted  bool
	projectRoot string
}

type taskCreatedMsg struct {
	id  string
	err error
}

func newCreateTaskModel(projectRoot string) createTaskModel {
	ti := textinput.New()
	ti.Placeholder = "Task title (required)"
	ti.Focus()
	ti.CharLimit = 200

	di := textinput.New()
	di.Placeholder = "Description (optional)"
	di.CharLimit = 500

	return createTaskModel{
		titleInput:  ti,
		descInput:   di,
		typeIdx:     0, // task
		prioIdx:     2, // P2
		activeField: fieldTitle,
		projectRoot: projectRoot,
	}
}

func (m createTaskModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m createTaskModel) Update(msg tea.Msg) (createTaskModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
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
			if m.activeField == fieldType || m.activeField == fieldPriority {
				// Enter on selector cycles forward
				if m.activeField == fieldType {
					m.typeIdx = (m.typeIdx + 1) % len(taskTypes)
				} else {
					m.prioIdx = (m.prioIdx + 1) % len(priorities)
				}
				return m, nil
			}
			// Enter on title or desc: submit if title is non-empty
			if strings.TrimSpace(m.titleInput.Value()) == "" {
				m.err = fmt.Errorf("title is required")
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
	projectRoot := m.projectRoot

	return func() tea.Msg {
		args := []string{"-w", projectRoot, "create", title, "-t", typ, "-p", prio}
		if desc != "" {
			args = append(args, "-d", desc)
		}
		out, err := exec.Command("td", args...).CombinedOutput()
		if err != nil {
			return taskCreatedMsg{err: fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)}
		}
		return taskCreatedMsg{id: strings.TrimSpace(string(out))}
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
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(highlight).Render("New Task"))
	b.WriteString("\n\n")

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
