package api

import "testing"

func TestRunOpen(t *testing.T) {
	for _, tc := range []struct {
		status string
		want   bool
	}{
		{"queued", true},
		{"running", true},
		{"passed", false},
		{"failed", false},
		{"timed_out", false},
		{"cancelled", false},
	} {
		if got := RunOpen(Run{Status: tc.status}); got != tc.want {
			t.Errorf("RunOpen(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}
