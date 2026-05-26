package prs

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/flo-at/sindri/internal/plugin"
	"github.com/flo-at/sindri/internal/styles"
)

const (
	pluginID   = "prs"
	pluginName = "prs"
	pluginIcon = "P"
)

type Plugin struct {
	ctx     *plugin.Context
	focused bool
	width   int
	height  int

	prs      []*store.PR
	selected int
	loading  bool
}

type refreshMsg struct{ prs []*store.PR }
type refreshTickMsg struct{}

func New() *Plugin { return &Plugin{} }

func (p *Plugin) ID() string   { return pluginID }
func (p *Plugin) Name() string { return pluginName }
func (p *Plugin) Icon() string { return pluginIcon }

func (p *Plugin) Init(ctx *plugin.Context) error {
	p.ctx = ctx
	return nil
}

func (p *Plugin) Start() tea.Cmd {
	return p.refresh()
}

func (p *Plugin) Stop() {}

func (p *Plugin) IsFocused() bool      { return p.focused }
func (p *Plugin) SetFocused(f bool)    { p.focused = f }
func (p *Plugin) FocusContext() string  { return "prs" }
func (p *Plugin) Commands() []plugin.Command { return nil }

func (p *Plugin) refresh() tea.Cmd {
	return func() tea.Msg {
		prs, _ := store.ListFor(p.ctx.ProjectRoot)
		return refreshMsg{prs: prs}
	}
}

func (p *Plugin) Update(msg tea.Msg) (plugin.Plugin, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		p.prs = msg.prs
		p.loading = false
		// Poll every 5 seconds
		return p, tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
			return refreshTickMsg{}
		})

	case refreshTickMsg:
		return p, p.refresh()

	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if p.selected < len(p.prs)-1 {
				p.selected++
			}
		case "k", "up":
			if p.selected > 0 {
				p.selected--
			}
		case "a":
			// Approve selected PR
			if p.selected < len(p.prs) {
				pr := p.prs[p.selected]
				if pr.Status == "open" {
					_ = exec.Command("gh", "pr", "review", pr.ID, "--approve").Run()
					return p, p.refresh()
				}
			}
		case "m":
			// Merge selected PR
			if p.selected < len(p.prs) {
				pr := p.prs[p.selected]
				if pr.Status == "approved" {
					_ = exec.Command("gh", "pr", "merge", pr.ID).Run()
					return p, p.refresh()
				}
			}
		case "r":
			return p, p.refresh()
		}
	}
	return p, nil
}

func (p *Plugin) View(width, height int) string {
	p.width = width
	p.height = height

	if len(p.prs) == 0 {
		hint := styles.Muted.Render("No PRs. Workers create PRs when they complete tasks.\n\nPress 'r' to refresh.")
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, hint)
	}

	var b strings.Builder

	// Header
	header := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary).Render("Pull Requests")
	b.WriteString(header + "\n")
	b.WriteString(styles.Muted.Render("a=approve  m=merge  r=refresh") + "\n\n")

	for i, pr := range p.prs {
		selected := i == p.selected

		// Status icon + color
		var icon string
		var statusStyle lipgloss.Style
		switch pr.Status {
		case "open":
			icon = "○"
			statusStyle = lipgloss.NewStyle().Foreground(styles.Warning)
		case "approved":
			icon = "✓"
			statusStyle = lipgloss.NewStyle().Foreground(styles.Success)
		case "merged":
			icon = "●"
			statusStyle = lipgloss.NewStyle().Foreground(styles.Primary)
		default:
			icon = "?"
			statusStyle = styles.Muted
		}

		line := fmt.Sprintf(" %s %s  %s → %s  %s",
			statusStyle.Render(icon),
			pr.Title,
			styles.Muted.Render(pr.Branch),
			styles.Muted.Render(pr.Base),
			statusStyle.Render(pr.Status),
		)

		if selected && p.focused {
			b.WriteString(styles.ListItemSelected.Width(width).Render(line))
		} else if selected {
			dimStyle := lipgloss.NewStyle().Background(styles.BgSecondary).Width(width)
			b.WriteString(dimStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")

		// Show body for selected PR
		if selected && pr.Body != "" {
			body := styles.Muted.Render("  " + strings.ReplaceAll(pr.Body, "\n", "\n  "))
			b.WriteString(body + "\n")
		}
	}

	return b.String()
}

