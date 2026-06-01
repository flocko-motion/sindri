package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReplay_BasicListAndFilter sanity-checks the replay engine end-to-end:
// build the simple fixture, capture the default list, cycle the filter to
// "all" (which reveals the closed task), then back. The captures must show
// the right items at each step.
func TestReplay_BasicListAndFilter(t *testing.T) {
	dir := t.TempDir()
	script := "(capture list-default) f (capture list-all) f (capture list-closed) f (capture list-back)"
	if err := Replay(script, SimpleFixture(), dir); err != nil {
		t.Fatalf("replay: %v", err)
	}

	readTxt := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name+".txt"))
		if err != nil {
			t.Fatalf("missing capture %s: %v", name, err)
		}
		return string(b)
	}
	containsAll := func(s string, subs ...string) bool {
		for _, sub := range subs {
			if !strings.Contains(s, sub) {
				return false
			}
		}
		return true
	}

	// Default filter (FilterOpen) hides closed items.
	def := readTxt("list-default")
	if !containsAll(def, "td-aaaaaa", "td-bbbbbb") {
		t.Errorf("default list missing open/in-progress items:\n%s", def)
	}
	if strings.Contains(def, "td-cccccc") {
		t.Errorf("default list should hide closed td-cccccc but didn't:\n%s", def)
	}

	// One 'f' press → FilterAll: everything visible.
	all := readTxt("list-all")
	if !containsAll(all, "td-aaaaaa", "td-bbbbbb", "td-cccccc") {
		t.Errorf("FilterAll missing some items:\n%s", all)
	}

	// Two presses → FilterClosed: only the closed task.
	closed := readTxt("list-closed")
	if !strings.Contains(closed, "td-cccccc") {
		t.Errorf("FilterClosed missing closed task:\n%s", closed)
	}
	if strings.Contains(closed, "td-aaaaaa") {
		t.Errorf("FilterClosed should hide open td-aaaaaa but didn't:\n%s", closed)
	}

	// Three presses → back to FilterOpen, same as default.
	back := readTxt("list-back")
	if strings.Contains(back, "td-cccccc") {
		t.Errorf("after cycling back, closed should be hidden again:\n%s", back)
	}

	// Captures should produce both .ansi and .txt variants.
	for _, name := range []string{"list-default", "list-all", "list-closed", "list-back"} {
		for _, ext := range []string{".ansi", ".txt"} {
			if _, err := os.Stat(filepath.Join(dir, name+ext)); err != nil {
				t.Errorf("missing capture file %s%s: %v", name, ext, err)
			}
		}
	}
}

// TestReplay_UnknownToken proves bad scripts fail loudly.
func TestReplay_UnknownToken(t *testing.T) {
	err := Replay("(frobnicate)", SimpleFixture(), t.TempDir())
	if err == nil {
		t.Fatalf("expected an error for unknown directive, got nil")
	}
	if !strings.Contains(err.Error(), "frobnicate") {
		t.Errorf("error should name the offending token: %v", err)
	}
}
