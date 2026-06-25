// package: tui / util
// type:    small shared helpers
// job:     the selector row type and tiny generic helpers used across the
//          tab/component files.
// limits:  tiny generic helpers only; no domain logic and no rendering of its
//          own (-> the tabs/components).
package tui

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
