package api

import (
	"testing"
	"time"
)

// TestRunTook covers the three cases the column must render: blank while queued (it has taken no
// time, and "0s" would read as instant rather than absent), the elapsed span while still running,
// and the full span once finished — cancelled and timed-out runs read the same as passed/failed,
// since RunTook only asks whether a finish time exists, never what the status says.
func TestRunTook(t *testing.T) {
	now := time.Now()
	fmtStamp := func(t time.Time) string { return t.Format(time.RFC3339) }

	if got := RunTook(Run{Status: "queued"}); got != "" {
		t.Errorf("a queued run (never started) should read blank, got %q", got)
	}

	running := Run{Status: "running", StartedAt: fmtStamp(now.Add(-90 * time.Second))}
	got, err := time.ParseDuration(RunTook(running))
	if err != nil {
		t.Fatalf("a running run's took should parse as a duration, got %q: %v", RunTook(running), err)
	}
	if got < 89*time.Second || got > 92*time.Second {
		t.Errorf("a running run should show ~90s elapsed (rounded), got %v", got)
	}

	for _, status := range []string{"passed", "failed", "timed_out", "cancelled"} {
		r := Run{
			Status:     status,
			StartedAt:  fmtStamp(now.Add(-5 * time.Minute)),
			FinishedAt: fmtStamp(now.Add(-1 * time.Minute)),
		}
		if got := RunTook(r); got != "4m0s" {
			t.Errorf("%s: RunTook = %q, want 4m0s (finished - started)", status, got)
		}
	}
}

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
