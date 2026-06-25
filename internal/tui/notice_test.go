package tui

import (
	"strings"
	"testing"
)

func TestStartupNoticeRendersAndDismisses(t *testing.T) {
	m := newModel(nil, nil, "")
	m.w, m.h = 120, 36
	m.noticeText = openspecMissingNotice

	view := m.View()
	if !strings.Contains(view, "⚠ warning") {
		t.Fatalf("notice should render a warning modal, got:\n%s", view)
	}
	if !strings.Contains(view, "@fission-ai/openspec") {
		t.Errorf("notice should include the install command, got:\n%s", view)
	}

	// Any key dismisses it; the dashboard then renders normally.
	tm, _ := m.Update(keyMsg("x"))
	if got := tm.(model).noticeText; got != "" {
		t.Fatalf("a key should dismiss the notice, still set to %q", got)
	}
	if strings.Contains(tm.View(), "⚠ warning") {
		t.Error("warning modal should be gone after dismissal")
	}
}
