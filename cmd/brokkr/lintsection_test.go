package main

import (
	"strings"
	"testing"
)

// TestWriteSectionBannersOnlyFindings pins what made a clean run seventeen lines of nothing: every
// linter printed a banner and a blank line whether or not it had anything to report.
func TestWriteSectionBannersOnlyFindings(t *testing.T) {
	for _, c := range []struct {
		what       string
		body       string
		bad        bool
		wantBanner bool
		wantBody   bool
	}{
		{"a linter with findings", "main.go:1: something\n", true, true, true},
		{"a linter with nothing to say", "", false, false, false},
		{"a passing linter's verdict is evidence, not a finding", "openspec: 21 passed\n", false, false, true},
		{"a failure with no detail still names itself", "", true, true, false},
	} {
		var sb strings.Builder
		writeSection(&sb, "openspec", []byte(c.body), c.bad)
		got := sb.String()
		if hasBanner := strings.Contains(got, "== openspec =="); hasBanner != c.wantBanner {
			t.Errorf("%s: banner = %v, want %v (output %q)", c.what, hasBanner, c.wantBanner, got)
		}
		if hasBody := c.body != "" && strings.Contains(got, strings.TrimSpace(c.body)); hasBody != c.wantBody {
			t.Errorf("%s: body kept = %v, want %v (output %q)", c.what, hasBody, c.wantBody, got)
		}
	}
}

// TestWriteSectionAddsNothingWhenSilent: a passing, silent linter must contribute not one byte, or
// seven of them still fill a screen.
func TestWriteSectionAddsNothingWhenSilent(t *testing.T) {
	var sb strings.Builder
	for _, name := range []string{"deadcode", "loc", "comments", "comment-length", "gofmt", "js"} {
		writeSection(&sb, name, nil, false)
	}
	if sb.Len() != 0 {
		t.Errorf("six quiet linters wrote %d bytes:\n%s", sb.Len(), sb.String())
	}
}
