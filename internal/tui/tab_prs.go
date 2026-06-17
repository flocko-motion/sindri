// package: tui / prs
// type:    ui (PRs tab)
// job:     the PRs tab content — the merge-intent list and the PR detail pane
//          (metadata + the linked task + the diff), the latter lazily fetched.
package tui

import (
	"fmt"
	"strings"
)

func (m model) prRows() []row {
	var out []row
	for _, p := range m.state.PRs {
		out = append(out, row{fmt.Sprintf("%-14s %-9s %-10s %s", p.ID, p.Status, p.Agent, p.Branch), p.ID})
	}
	return out
}

func (m model) prDetailLines() []string {
	id := m.selID()
	if id == "" {
		return []string{dimStyle.Render("(no PR)")}
	}
	d := m.prDetail
	if d.PR.ID != id {
		return []string{id, dimStyle.Render("(loading…)")}
	}
	ls := []string{
		fmt.Sprintf("%s   [%s]   by %s", d.PR.ID, d.PR.Status, d.PR.Agent),
		fmt.Sprintf("task: %s  %s (%s)", d.Task.ID, d.Task.Title, d.Task.Status),
		fmt.Sprintf("branch %s → %s", d.PR.Branch, d.PR.Base),
	}
	if d.PR.Feedback != "" {
		ls = append(ls, "feedback: "+d.PR.Feedback)
	}
	ls = append(ls, "", "── diff ──")
	if strings.TrimSpace(d.Diff) == "" {
		ls = append(ls, dimStyle.Render("(no diff)"))
	} else {
		ls = append(ls, strings.Split(strings.TrimRight(d.Diff, "\n"), "\n")...)
	}
	return ls
}
