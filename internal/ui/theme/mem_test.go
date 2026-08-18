package theme

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/flo-at/sindri/internal/api"
)

const gib = int64(1) << 30

// fleet is a machine with 10 GiB free and room for three more agents of the default size.
func fleet() api.FleetMemory {
	return api.FleetMemory{
		UsedBytes: 6 * gib, TotalBytes: 16 * gib, AgentBytes: 3 * gib, Fits: 3, Basis: "in use",
	}
}

// TestFleetLineAnswersWhetherAnotherAgentFits: the line exists for that question, so it must name
// what is free and how many agents that is — in the size it counted, which the reader does not
// otherwise know.
func TestFleetLineAnswersWhetherAnotherAgentFits(t *testing.T) {
	got := FleetLine(fleet())
	for _, want := range []string{"10 GiB free", "room for 3 more agents", "3 GiB each", "in use"} {
		if !strings.Contains(got, want) {
			t.Errorf("FleetLine = %q, want it to contain %q", got, want)
		}
	}
}

// TestFleetLineSaysWhenNothingFits: zero is the reading that matters most, and "room for 0 more
// agents" is a sentence a skimming eye reads as room.
func TestFleetLineSaysWhenNothingFits(t *testing.T) {
	m := fleet()
	m.UsedBytes, m.Fits = 15*gib, 0
	if got := FleetLine(m); !strings.Contains(got, "no room for another agent") {
		t.Errorf("FleetLine = %q, want it to say plainly that nothing fits", got)
	}
}

// TestUnmeasuredRendersAsNothing: a runtime that could not answer must not render as a full
// machine — the line says so in words, and the header badge takes no space at all.
func TestUnmeasuredRendersAsNothing(t *testing.T) {
	if got := FleetLine(api.FleetMemory{}); !strings.Contains(got, "not reported") {
		t.Errorf("FleetLine of an unknown figure = %q, want it to say it is not reported", got)
	}
	if got := FleetBadge(api.FleetMemory{}, 80); got != "" {
		t.Errorf("FleetBadge of an unknown figure = %q, want nothing", got)
	}
}

// TestBadgeDegradesToFitInTheSpaceGiven: the header hands the badge whatever the tabs left over,
// which on a narrow terminal is very little. It must always fit, shed the working before the
// answer, and disappear rather than overflow.
func TestBadgeDegradesToFitInTheSpaceGiven(t *testing.T) {
	m := fleet()
	for _, width := range []int{80, 40, 30, 20, 12, 6, 5, 0} {
		got := FleetBadge(m, width)
		if lipgloss.Width(got) > width {
			t.Errorf("FleetBadge(%d) = %q, %d cells wide", width, got, lipgloss.Width(got))
		}
		if got != "" && !strings.Contains(got, "3") {
			t.Errorf("FleetBadge(%d) = %q, want the fit count kept in every form", width, got)
		}
	}
	if got := FleetBadge(m, 5); got != "" {
		t.Errorf("FleetBadge(5) = %q, want nothing — the tabs own that space", got)
	}
	if got := FleetBadge(m, 40); !strings.Contains(got, "█") || !strings.Contains(got, "10 GiB free") {
		t.Errorf("FleetBadge(40) = %q, want the meter and the free figure where there is room", got)
	}
}
