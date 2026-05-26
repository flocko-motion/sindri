package prs

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/plugin"
	"github.com/flo-at/sindri/internal/styles"
)

const (
	pluginID   = "prs"
	pluginName = "prs"
	pluginIcon = "P"
)

type PR struct {
	ID        string `json:"id"`
	Branch    string `json:"branch"`
	Base      string `json:"base"`
	Status    string `json:"status"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type Plugin struct {
	ctx     *plugin.Context
	focused bool
	width   int
	height  int

	prs      []PR
	selected int
	loading  bool
}

type refreshMsg struct{ prs []PR }
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
		prs := loadPRs(p.ctx.ProjectRoot)
		return refreshMsg{prs: prs}
	}
}

func (p *Plugin) Update(msg tea.Msg) (plugin.Plugin, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		p.prs = msg.prs
		p.loading = false
		// Poll every 5 seconds
		return p, tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
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

func loadPRs(projectRoot string) []PR {
	cmd := exec.Command("gh", "pr", "list")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	// gh pr list outputs: ID  STATUS  BRANCH → BASE
	// But we can also read the JSON files directly
	var prs []PR
	prDir := projectRoot + "/.git/pr"
	entries, err := exec.Command("ls", prDir).Output()
	if err != nil {
		return nil
	}
	for _, name := range strings.Split(strings.TrimSpace(string(entries)), "\n") {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := exec.Command("cat", prDir+"/"+name).Output()
		if err != nil {
			continue
		}
		var pr PR
		if json.Unmarshal(data, &pr) == nil {
			prs = append(prs, pr)
		}
	}
	_ = out
	return prs
}
