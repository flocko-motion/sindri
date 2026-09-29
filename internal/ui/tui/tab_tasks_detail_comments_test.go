package tui

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/api"
)

// TestCommentThreadNamesItsSource: the thread showed author and time but never which source the
// comment belongs to. "github" means it came from or went to the upstream issue, so it says who
// else has already read it — the CLI's `task info` prints it, and the agent-facing view now does
// too, which left the TUI the one surface answering a narrower question.
func TestCommentThreadNamesItsSource(t *testing.T) {
	items := commentItems([]api.Comment{
		{Source: "github", Author: "dain", Body: "Repro on 1.26 too.", CreatedAt: "2026-08-10T09:30:00Z"},
		{Source: "td", Author: "bombur", Body: "Root cause is the cache key.", CreatedAt: "2026-08-11T14:05:00Z"},
	})
	got := strings.Join(itemTexts(items), "\n")
	for _, want := range []string{"dain", "github", "Repro on 1.26 too.", "bombur", "td", "Root cause is the cache key."} {
		if !strings.Contains(got, want) {
			t.Errorf("the thread is missing %q:\n%s", want, got)
		}
	}
}

// TestEmptyThreadRendersNothing: the heading is built from the comment count, so an empty thread
// must produce no items at all rather than a "comments (0)" line on every task nobody has replied
// to. The detail pane is read constantly; a heading that is always there teaches skimming.
func TestEmptyThreadRendersNothing(t *testing.T) {
	if items := commentItems(nil); len(items) != 0 {
		t.Errorf("an empty thread rendered %d items: %q", len(items), itemTexts(items))
	}
}
