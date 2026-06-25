package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// plain strips ANSI styling so tests assert on the visible text only.
func plain(s string) string { return ansi.Strip(s) }

func TestRenderDiffLineNumbers(t *testing.T) {
	diff := `diff --git a/f.go b/f.go
index 111..222 100644
--- a/f.go
+++ b/f.go
@@ -10,4 +10,5 @@ func F() {
 ctx-a
-removed
+added-1
+added-2
 ctx-b`
	lines := renderDiff(diff)
	var got []string
	for _, l := range lines {
		got = append(got, plain(l))
	}

	// Context advances both counters; an addition takes the new-file number, a
	// removal takes the old-file number (an editor's diff view).
	want := []string{
		"   10   ctx-a",   // context: old 10 / new 10
		"   11 - removed", // removal: old line 11 (ctx-a was old 10)
		"   11 + added-1", // addition: new line 11 (ctx-a was new 10)
		"   12 + added-2", // addition: new line 12
		"   13   ctx-b",   // context: new line 13 (10 + 1 ctx + 2 adds)
	}
	// Find the body lines (skip the file/hunk headers).
	var body []string
	for _, g := range got {
		if strings.Contains(g, "ctx-") || strings.Contains(g, "removed") || strings.Contains(g, "added-") {
			body = append(body, g)
		}
	}
	if len(body) != len(want) {
		t.Fatalf("expected %d body lines, got %d:\n%s", len(want), len(body), strings.Join(body, "\n"))
	}
	for i := range want {
		if body[i] != want[i] {
			t.Errorf("body line %d:\n  got  %q\n  want %q", i, body[i], want[i])
		}
	}
}

func TestRenderDiffColorsAddAndRemove(t *testing.T) {
	// Other tests in this package force the Ascii (colourless) profile; opt this
	// one into colour so the background SGR codes are actually emitted, then
	// restore so test order stays irrelevant.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prev)

	diff := "@@ -1,1 +1,1 @@\n-old\n+new"
	lines := renderDiff(diff)
	var add, del string
	for _, l := range lines {
		if strings.Contains(plain(l), "new") {
			add = l
		}
		if strings.Contains(plain(l), "old") {
			del = l
		}
	}
	// The styled segment must carry the configured background colours (22 green /
	// 52 red render to SGR 48;5;22 / 48;5;52).
	if !strings.Contains(add, "48;5;22") {
		t.Errorf("addition should have the green background, got %q", add)
	}
	if !strings.Contains(del, "48;5;52") {
		t.Errorf("removal should have the red background, got %q", del)
	}
}
