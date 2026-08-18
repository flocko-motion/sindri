package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/theme"
)

// TestPRKindMarkers: the task-tree PR marker distinguishes a final (task-done) PR from
// an interim (mid-task) one, and a row with no PR carries neither. A kindless PR (the
// historical default) reads as final. Asserted against the shared marks rather than the
// characters they happen to be, so the pair can be redrawn without rewriting the rule.
func TestPRKindMarkers(t *testing.T) {
	cases := []struct {
		row  api.TaskRow
		want string // the mark the column must carry ("" = neither)
	}{
		{api.TaskRow{PR: "pr-1", PRKind: "final"}, theme.MarkPRFinal},
		{api.TaskRow{PR: "pr-1", PRKind: "interim"}, theme.MarkPRInterim},
		{api.TaskRow{PR: "pr-1", PRKind: ""}, theme.MarkPRFinal}, // kindless → final
		{api.TaskRow{PR: "", PRKind: ""}, ""},                    // no PR → no marker
	}
	for _, c := range cases {
		got := taskMarks(false, prMarkKind(c.row))
		hasFinal := strings.Contains(got, theme.MarkPRFinal)
		hasInterim := strings.Contains(got, theme.MarkPRInterim)
		switch c.want {
		case theme.MarkPRFinal:
			if !hasFinal || hasInterim {
				t.Errorf("PR %q/%q: marker=%q, want the final mark", c.row.PR, c.row.PRKind, got)
			}
		case theme.MarkPRInterim:
			if !hasInterim || hasFinal {
				t.Errorf("PR %q/%q: marker=%q, want the interim mark", c.row.PR, c.row.PRKind, got)
			}
		default:
			if hasFinal || hasInterim {
				t.Errorf("PR %q/%q: marker=%q, want no PR marker", c.row.PR, c.row.PRKind, got)
			}
		}
	}
}

// TestTheMarkerColumnAlwaysFillsItsWidth: the column is padded so every title starts at the same
// column, whatever the row carries. It is measured from the marks themselves, so a glyph swap that
// changed the count would be caught here rather than in a crooked tree.
func TestTheMarkerColumnAlwaysFillsItsWidth(t *testing.T) {
	for _, assigned := range []bool{false, true} {
		for _, kind := range []string{"", "final", "interim"} {
			got := taskMarks(assigned, kind)
			if w := ansi.StringWidth(got); w != marksW {
				t.Errorf("assigned=%v kind=%q: marks %q are %d cells, the column is %d",
					assigned, kind, got, w, marksW)
			}
		}
	}
	// Both marks at once is the widest a row gets, and the width is exactly that — a column
	// wider than its content is padding nobody asked for, narrower would truncate a mark.
	if w := ansi.StringWidth(taskMarks(true, "final")); w != marksW {
		t.Errorf("a row carrying both marks is %d cells against a column of %d", w, marksW)
	}
}

// TestPRKindLabel: the human-readable kind label for the detail pane / CLI.
func TestPRKindLabel(t *testing.T) {
	if !strings.Contains(prKindLabel("interim"), "interim") {
		t.Errorf("interim label should say interim, got %q", prKindLabel("interim"))
	}
	if !strings.Contains(prKindLabel(""), "final") {
		t.Errorf("kindless label should read as final, got %q", prKindLabel(""))
	}
}
