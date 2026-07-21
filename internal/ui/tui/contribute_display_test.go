package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub"
)

// TestPRKindMarkers: the task-tree PR marker distinguishes a final (task-done) PR from
// an interim (mid-task) one, and a row with no PR carries neither. A kindless PR (the
// historical default) reads as final.
func TestPRKindMarkers(t *testing.T) {
	cases := []struct {
		row  hub.TaskRow
		want string // substring the marker column must contain ("" = neither ◆ nor ◇)
	}{
		{hub.TaskRow{PR: "pr-1", PRKind: "final"}, "◆"},
		{hub.TaskRow{PR: "pr-1", PRKind: "interim"}, "◇"},
		{hub.TaskRow{PR: "pr-1", PRKind: ""}, "◆"}, // kindless → final
		{hub.TaskRow{PR: "", PRKind: ""}, ""},      // no PR → no marker
	}
	for _, c := range cases {
		got := taskMarks(false, prMarkKind(c.row))
		hasFinal, hasInterim := strings.Contains(got, "◆"), strings.Contains(got, "◇")
		switch c.want {
		case "◆":
			if !hasFinal || hasInterim {
				t.Errorf("PR %q/%q: marker=%q, want ◆", c.row.PR, c.row.PRKind, got)
			}
		case "◇":
			if !hasInterim || hasFinal {
				t.Errorf("PR %q/%q: marker=%q, want ◇", c.row.PR, c.row.PRKind, got)
			}
		default:
			if hasFinal || hasInterim {
				t.Errorf("PR %q/%q: marker=%q, want no PR marker", c.row.PR, c.row.PRKind, got)
			}
		}
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
