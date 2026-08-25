// package: tui / repos tab
// type:    ui (the Repos tab — the registry surface)
// job:     list the repos the hub tracks (the TUI counterpart of `sindri repo …`):
// each in its colour, with agent count and live/current markers; enter
// switches to a repo, D forgets it (agent-guarded, files kept). Detail shows
// the repo's path, agents, and PRs.
// limits:  rendering + action wiring; the registry lives in the hub (repo.go).
package tui

import (
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/table"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// repoRows lists the registered repos (switcher order: live-agents-first → recency →
// alphabetical), each name in its repo colour with an agent count and markers. The
// row id is the repo tag.
// repoTable is the Repos list's columns. The counts used to carry their own word on every row
// ("3 agents"); with a label over the column the word belongs there instead, once.
var repoTable = table.Table{
	{Label: "repo", Width: 20},
	{Label: "agents", Width: 6, Right: true},
	{Label: "path"},
}

func (m model) repoRows() []row {
	var out []row
	for _, p := range m.switcherOrder() {
		label := m.repoName(p.Tag)
		if m.repoHasLiveAgent(p.Tag) {
			label += " ●"
		}
		if p.Path == m.root {
			label += " ✓"
		}
		out = append(out, row{repoTable.Line(
			table.Cell{Text: label, Style: m.repoStyle(p.Tag).Render},
			table.Cell{Text: strconv.Itoa(m.repoAgentCount(p.Tag))},
			table.Cell{Text: p.Path},
		), p.Tag})
	}
	return m.listing(repoTable, nil, out)
}

// repoAgentCount is how many agents the repo has on its roster (from the board).
func (m model) repoAgentCount(tag string) int {
	n := 0
	for _, a := range m.state.Agents {
		if a.Project == tag {
			n++
		}
	}
	return n
}

// repoDetailLines renders the selected repo's detail: path, tag, its agents, and its
// PRs — all from the board snapshot (no fetch).
func (m model) repoDetailLines() []string { return itemTexts(m.repoItems()) }

// repoItems is the selected repo's detail; each of its agents and open PRs is its own
// cross-reference line, so the roster and the queue can be reached one row at a time rather than
// read as a single joined string nothing can select.
func (m model) repoItems() []metaItem {
	tag := m.selID()
	if tag == "" {
		return []metaItem{{text: dimStyle.Render("(no repos)")}}
	}
	var path, last string
	for _, p := range m.state.Projects {
		if p.Tag == tag {
			path, last = p.Path, p.LastUsed
		}
	}
	items := []metaItem{
		{text: "repo:   " + m.repoName(tag)},
		{text: "path:   " + path},
		{text: "tag:    " + tag},
	}
	if last != "" {
		items = append(items, metaItem{text: "used:   " + shortAge(last) + " ago"})
	}
	items = append(items, metaItem{text: ""}, metaItem{text: "agents:"})
	items = append(items, repoAgentItems(m.state.Agents, tag)...)
	items = append(items, metaItem{text: ""}, metaItem{text: "open prs:"})
	items = append(items, repoPRItems(m.state.PRs, tag)...)
	for _, l := range gateLines(m.state.RepoDocs[tag]) {
		items = append(items, metaItem{text: l})
	}
	for _, l := range archLines(m.state.RepoDocs[tag]) {
		items = append(items, metaItem{text: l})
	}
	items = append(items, metaItem{text: ""}, metaItem{text: dimStyle.Render("enter switch · E config · D forget")})
	return items
}

// repoAgentItems lists a repo's agents as cross-references, "  -" for none.
func repoAgentItems(agents []api.AgentView, tag string) []metaItem {
	var out []metaItem
	for _, a := range agents {
		if a.Project == tag {
			out = append(out, metaItem{text: "  " + a.Name + " (" + a.Status + ")", kind: "agent", value: a.Name})
		}
	}
	if len(out) == 0 {
		return []metaItem{{text: "  -"}}
	}
	return out
}

// repoPRItems lists a repo's still-open PRs as cross-references, "  -" for none.
func repoPRItems(prs []api.PR, tag string) []metaItem {
	var out []metaItem
	for _, p := range prs {
		if p.Project == tag && p.Status != "merged" {
			out = append(out, metaItem{text: "  " + p.ID + " " + p.Status, kind: "pr", value: p.ID})
		}
	}
	if len(out) == 0 {
		return []metaItem{{text: "  -"}}
	}
	return out
}

// repoActionable is the focusable subset of the repo detail (its agents and open PRs).
func (m model) repoActionable() []metaItem {
	var out []metaItem
	for _, it := range m.repoItems() {
		if it.kind != "" {
			out = append(out, it)
		}
	}
	return out
}

// archLines renders the repo's architecture-doc situation. This tab IS the UI for
// .sindri/config.yaml, so a repo that never told sindri where its architecture lives
// should see the gap here — the hub no longer seeds a placeholder to make the point, and
// a silent detail pane would hide a real quality loss (agents briefed without any
// architecture). A repo in good shape gets one confirming line and no nagging.
func archLines(st api.RepoDocState) []string {
	switch {
	case st.Readable && st.Set:
		return []string{"arch:   " + st.Doc}
	case st.Readable:
		return []string{"arch:   " + st.Doc + dimStyle.Render("  (default)")}
	case st.Advice == "":
		return nil // no snapshot for this repo (older hub) — say nothing rather than guess
	}
	return []string{"", stWarn.Render(warnGlyph + " no architecture doc"), dimStyle.Render("agents get no architecture brief — press E to set `architecture`")}
}

// gateLines renders the repo's quality gate, for the same reason archLines renders its doc — except
// this one is REQUIRED: an ungated repo cannot submit at all, so its absence is the loudest thing
// this pane can say about a repo.
func gateLines(st api.RepoDocState) []string {
	switch {
	case st.GateOK:
		return []string{"gate:   " + st.Gate}
	case st.GateAdvice == "":
		return nil // no snapshot for this repo (older hub) — say nothing rather than guess
	case st.Gate != "":
		return []string{"", stWarn.Render(warnGlyph + " gate " + st.Gate + " is missing"), dimStyle.Render("every submit refuses until that script is in the repo")}
	}
	return []string{"", stWarn.Render(warnGlyph + " no quality gate"), dimStyle.Render("nothing can be submitted from this repo — press E to set `verify`")}
}

// openColorChoice opens a picker of colour swatches for a repo: "default" (the
// hash-derived hue) plus each palette choice rendered in its own bright shade, so the
// list is visual. Selecting one pins it in the registry (per-machine display pref).
func (m *model) openColorChoice(tag string) {
	cl := m.cl
	opts := []string{"default (auto)"}
	vals := []string{"0"}
	for i := 1; i <= theme.NRepoColors; i++ {
		swatch := repoStyleFor(tag, i).Render("████")
		opts = append(opts, fmt.Sprintf("%s  colour %d", swatch, i))
		vals = append(vals, strconv.Itoa(i))
	}
	m.choice = choiceModalState{
		active: true, title: "colour for " + m.repoName(tag), options: opts, values: vals,
		apply: func(v string) tea.Cmd {
			n, _ := strconv.Atoi(v)
			return mutateThenRefresh(cl, func() error { return cl.SetRepoColor(tag, n) })
		},
	}
}

// openForgetChoice confirms forgetting a repo (it deletes the repo's agents and drops
// the registry row; the repo's files stay). Guarded behind a yes/no like agent delete.
func (m *model) openForgetChoice(tag, name string) {
	cl := m.cl
	m.choice = choiceModalState{
		active: true, title: "forget repo " + name + "? (deletes its agents; files kept)",
		options: []string{"cancel", "forget"}, values: []string{"cancel", "forget"},
		apply: func(v string) tea.Cmd {
			if v != "forget" {
				return nil
			}
			return mutateThenRefresh(cl, func() error { return cl.RepoForget(tag) })
		},
	}
}
