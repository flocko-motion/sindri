// package: tui / diff
// type:    ui (diff rendering)
// job:     turn a unified git diff into display lines for the PRs content pane —
//          a right-aligned line-number gutter, a +/-/· marker, and the content,
//          with additions on a green background and removals on red.
// limits:  pure formatting of a diff string; the colours live in theme.go and
//          the scroll/clip is the pane's job (-> component_pane.go).
package tui

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// renderDiff turns a unified git diff into display lines. Each body line carries
// the line number it has in the file an editor would show it at — the new-file
// number for additions and context, the old-file number for removals — then a
// +/-/space marker, then the content. Additions are on a green background,
// removals on red (the classic diff colours); hunk and file headers are tinted.
func renderDiff(diff string) []string {
	var out []string
	oldLn, newLn := 0, 0
	sc := bufio.NewScanner(strings.NewReader(diff))
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // diffs carry long lines
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "@@"):
			oldLn, newLn = parseHunkStarts(line)
			out = append(out, diffHunkStyle.Render(line))
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"),
			strings.HasPrefix(line, "diff --git"), strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "new file"), strings.HasPrefix(line, "deleted file"),
			strings.HasPrefix(line, "rename "), strings.HasPrefix(line, "similarity "),
			strings.HasPrefix(line, "old mode"), strings.HasPrefix(line, "new mode"),
			strings.HasPrefix(line, "\\"): // "\ No newline at end of file"
			out = append(out, diffMetaStyle.Render(line))
		case strings.HasPrefix(line, "+"):
			out = append(out, dimStyle.Render(gutter(newLn))+diffAddStyle.Render(" + "+line[1:]))
			newLn++
		case strings.HasPrefix(line, "-"):
			out = append(out, dimStyle.Render(gutter(oldLn))+diffDelStyle.Render(" - "+line[1:]))
			oldLn++
		default: // context (leading space) or a blank line inside a hunk
			content := line
			if strings.HasPrefix(line, " ") {
				content = line[1:]
			}
			out = append(out, dimStyle.Render(gutter(newLn))+"   "+content)
			oldLn++
			newLn++
		}
	}
	return out
}

// gutter right-aligns a line number into a fixed-width column so content lines
// up regardless of the number's magnitude.
func gutter(n int) string { return fmt.Sprintf("%5d", n) }

// parseHunkStarts reads the starting old/new line numbers from a hunk header
// `@@ -old[,n] +new[,n] @@`, defaulting to 1 if the header is malformed.
func parseHunkStarts(h string) (oldLn, newLn int) {
	oldLn, newLn = 1, 1
	for _, f := range strings.Fields(h) {
		switch {
		case strings.HasPrefix(f, "-"):
			oldLn = leadingInt(f[1:])
		case strings.HasPrefix(f, "+"):
			newLn = leadingInt(f[1:])
		}
	}
	return
}

// leadingInt parses the number before an optional ",count" suffix (e.g. "42,7").
func leadingInt(s string) int {
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(s)
	return n
}
