// package: tui / util
// type:    small shared helpers
// job:     the selector row type and tiny generic helpers used across the
//          tab/component files.
// limits:  tiny generic helpers only; no domain logic and no rendering of its
//          own (-> the tabs/components).
package tui

import "path/filepath"

// repoName maps a project's repoTag to its short repo name (its path's basename),
// falling back to the tag when the project isn't in the board's registry.
func (m model) repoName(tag string) string {
	for _, p := range m.state.Projects {
		if p.Tag == tag {
			return filepath.Base(p.Path)
		}
	}
	return tag
}

// currentRepo returns the selected repo's short name and tag (resolved from m.root),
// or ("","") when nothing matches — the source for the persistent header indicator.
func (m model) currentRepo() (name, tag string) {
	for _, p := range m.state.Projects {
		if p.Path == m.root {
			return filepath.Base(p.Path), p.Tag
		}
	}
	return "", ""
}

// row is one selector line: display text + the id it selects ("" = not selectable).
type row struct {
	text string
	id   string
}

func rowTexts(rows []row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.text
	}
	return out
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
