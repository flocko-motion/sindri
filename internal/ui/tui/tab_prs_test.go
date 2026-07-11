package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestWrapMetaWrapsPlainKeepsActionable(t *testing.T) {
	long := "this is a long history payload that should wrap across several lines rather than truncate"
	items := []metaItem{
		{text: "agent:  brokkr", kind: "agent", value: "brokkr"}, // actionable: stays one line
		{text: ""},        // spacer: stays one line
		{text: long},      // plain: must wrap
	}
	out := wrapMeta(items, 20)

	// The actionable item survives untouched and as a single entry.
	var actionable int
	for _, it := range out {
		if it.kind == "agent" {
			actionable++
			if it.text != "agent:  brokkr" {
				t.Errorf("actionable item altered: %q", it.text)
			}
		}
	}
	if actionable != 1 {
		t.Fatalf("expected exactly one actionable item, got %d", actionable)
	}

	// The long line became several entries, each within the width.
	var plainLines int
	for _, it := range out {
		if it.kind != "" || it.text == "" {
			continue
		}
		plainLines++
		if w := ansi.StringWidth(strings.TrimRight(it.text, " ")); w > 20 {
			t.Errorf("wrapped line exceeds width 20 (%d): %q", w, it.text)
		}
	}
	if plainLines < 2 {
		t.Fatalf("long text should wrap to >=2 lines, got %d", plainLines)
	}
}
