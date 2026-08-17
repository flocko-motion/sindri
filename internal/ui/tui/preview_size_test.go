package tui

import "testing"

// TestPreviewSizeMatchesTheRenderedPane: launchCmd sizes a fresh session off previewSize, and it
// must return exactly the width/height agentsBody actually renders the live pane at — a mismatch
// would size a session for a preview it doesn't get.
func TestPreviewSizeMatchesTheRenderedPane(t *testing.T) {
	m := newModel(nil, nil, "/r/sindri")
	m.tab = 1
	m.w, m.h = 150, 30 // wide: the detail column shows
	if !m.showDetail() {
		t.Fatal("precondition: expected the detail column to show at this width")
	}

	wantW := m.w - m.agentDetailWidth() - 1
	wantH := max(1, m.bodyHeight()-m.agentListHeight()-1)
	if gotW, gotH := m.previewSize(); gotW != wantW || gotH != wantH {
		t.Errorf("previewSize() = (%d, %d), want (%d, %d)", gotW, gotH, wantW, wantH)
	}
}

// TestPreviewSizeFillsTheWidthWithNoDetailColumn: narrow enough (or §-hidden) that the detail
// column is gone, the live pane — and so the session it should be created at — takes the full width.
func TestPreviewSizeFillsTheWidthWithNoDetailColumn(t *testing.T) {
	m := newModel(nil, nil, "/r/sindri")
	m.tab = 1
	m.w, m.h = 70, 20 // narrow: wide() is false, so showDetail() is false regardless of hideDetail
	if m.showDetail() {
		t.Fatal("precondition: expected the detail column to be hidden at this width")
	}
	if gotW, _ := m.previewSize(); gotW != m.w {
		t.Errorf("previewSize() width = %d, want the full %d with no detail column", gotW, m.w)
	}
}
