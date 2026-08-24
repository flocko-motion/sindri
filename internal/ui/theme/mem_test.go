package theme

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/api"
)

const gib = int64(1) << 30

// fleet is a machine with 6 GiB used of 16 GiB.
func fleet() api.FleetMemory {
	return api.FleetMemory{UsedBytes: 6 * gib, TotalBytes: 16 * gib, Basis: "in use"}
}

// TestFleetLineShowsUsageAndWorkload: the line names the cost against the ceiling, then how much of
// the roster is actually up — the question that matters once agents can be stopped and started on
// demand.
func TestFleetLineShowsUsageAndWorkload(t *testing.T) {
	got := FleetLine(fleet(), 3, 5)
	for _, want := range []string{"6 GiB / 16 GiB in use", "3 of 5 agents running"} {
		if !strings.Contains(got, want) {
			t.Errorf("FleetLine = %q, want it to contain %q", got, want)
		}
	}
}

// TestFleetLineSaysWhenAllRunning: every agent up is its own phrase, not "5 of 5".
func TestFleetLineSaysWhenAllRunning(t *testing.T) {
	if got := FleetLine(fleet(), 5, 5); !strings.Contains(got, "all 5 agents running") {
		t.Errorf("FleetLine = %q, want it to say plainly that all agents are running", got)
	}
}

// TestFleetLineSaysWhenNoneRegistered: an empty roster is not zero of zero running.
func TestFleetLineSaysWhenNoneRegistered(t *testing.T) {
	if got := FleetLine(fleet(), 0, 0); !strings.Contains(got, "no agents registered") {
		t.Errorf("FleetLine = %q, want it to say no agents are registered", got)
	}
}

// TestUnmeasuredRendersAsNothing: a runtime that could not answer must not render as a full
// machine — the line says so in words, and the header badge takes no space at all.
func TestUnmeasuredRendersAsNothing(t *testing.T) {
	if got := FleetLine(api.FleetMemory{}, 0, 0); !strings.Contains(got, "not reported") {
		t.Errorf("FleetLine of an unknown figure = %q, want it to say it is not reported", got)
	}
	if got := FleetBadge(api.FleetMemory{}, 0, 0, 80); got != "" {
		t.Errorf("FleetBadge of an unknown figure = %q, want nothing", got)
	}
}

// TestBadgeDegradesToFitInTheSpaceGiven: the header hands the badge whatever the tabs left over,
// which on a narrow terminal is very little. It must always fit, shed the working before the
// answer, and disappear rather than overflow.
func TestBadgeDegradesToFitInTheSpaceGiven(t *testing.T) {
	m := fleet()
	for _, width := range []int{80, 45, 40, 28, 20, 12, 11, 10, 0} {
		got := FleetBadge(m, 3, 5, width)
		if lipgloss.Width(got) > width {
			t.Errorf("FleetBadge(%d) = %q, %d cells wide", width, got, lipgloss.Width(got))
		}
		if got != "" && !strings.Contains(got, "3/5") {
			t.Errorf("FleetBadge(%d) = %q, want the running count kept in every form", width, got)
		}
	}
	if got := FleetBadge(m, 3, 5, 10); got != "" {
		t.Errorf("FleetBadge(10) = %q, want nothing — the tabs own that space", got)
	}
	if got := FleetBadge(m, 3, 5, 45); !strings.Contains(got, "█") || !strings.Contains(got, "6 GiB") {
		t.Errorf("FleetBadge(45) = %q, want the meter and the used figure where there is room", got)
	}
}
