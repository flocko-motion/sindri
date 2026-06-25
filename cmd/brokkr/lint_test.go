package main

import (
	"errors"
	"strings"
	"testing"
)

func TestLintOutcome(t *testing.T) {
	cases := []struct {
		name     string
		fn       func() (bool, error)
		wantCode int
		wantMark string   // marker line expected
		wantHas  []string // other substrings expected in output
	}{
		{"clean", func() (bool, error) { return false, nil }, 0, "=== EXIT 0 ===", nil},
		{"violations", func() (bool, error) { return true, nil }, 1, "=== EXIT 1 ===", nil},
		{"error", func() (bool, error) { return false, errors.New("boom") }, 1, "=== EXIT 1 ===", []string{"error: boom"}},
		{"panic", func() (bool, error) { panic("kaboom") }, 1, "=== EXIT 1 ===", []string{"panic: kaboom"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sb strings.Builder
			got := lintOutcome(&sb, tc.fn)
			out := sb.String()
			if got != tc.wantCode {
				t.Errorf("code = %d, want %d", got, tc.wantCode)
			}
			if !strings.Contains(out, tc.wantMark) {
				t.Errorf("missing marker %q in:\n%s", tc.wantMark, out)
			}
			for _, h := range tc.wantHas {
				if !strings.Contains(out, h) {
					t.Errorf("missing %q in:\n%s", h, out)
				}
			}
		})
	}
}
