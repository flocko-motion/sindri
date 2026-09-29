package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/flo-at/sindri/internal/api"
)

// headroom is a machine with 6 GiB used of 16 GiB.
func headroom() api.FleetMemory {
	const gib = int64(1) << 30
	return api.FleetMemory{UsedBytes: 6 * gib, TotalBytes: 16 * gib, Basis: "in use"}
}

// TestHeaderShowsHeadroomWhereThereIsRoom: the figure is in the header so the answer to "is another
// agent already able to run right now" is in view from every tab, without opening anything.
func TestHeaderShowsHeadroomWhereThereIsRoom(t *testing.T) {
	labels := []string{"1 Repos", "2 Agents", "3 Tasks", "4 PRs", "5 Room"}
	for _, repo := range []string{"", "ranke-db"} {
		got := headerBar(labels, 1, 120, repo, "rdb", 0, headroom(), 3, 5)
		if !strings.Contains(got, "3/5") {
			t.Errorf("repo=%q: header = %q, want the running count in it", repo, got)
		}
		if !strings.Contains(got, "6 GiB") {
			t.Errorf("repo=%q: header = %q, want what is used in it", repo, got)
		}
	}
}

// TestHeaderYieldsItsSpaceToTheTabs is the width the header has to degrade in. A memory figure
// that pushed a tab off the edge would cost the user more than it told them, so on a terminal with
// no room to spare the badge goes and the bar still fits.
func TestHeaderYieldsItsSpaceToTheTabs(t *testing.T) {
	labels := []string{"1 Repos", "2 Agents", "3 Tasks", "4 PRs", "5 Room"}
	for _, width := range []int{60, 70, 80, 100, 120, 200} {
		for _, repo := range []string{"", "ranke-db"} {
			bar := headerBar(labels, 1, width, repo, "rdb", 0, headroom(), 3, 5)
			if got := lipgloss.Width(bar); got > width && width >= plainHeaderWidth(labels, repo) {
				t.Errorf("w=%d repo=%q: header is %d cells wide:\n%q", width, repo, got, bar)
			}
		}
	}
	// The tabs alone already fill this one: nothing is left to say the figure in.
	if bar := headerBar(labels, 1, 46, "ranke-db", "rdb", 0, headroom(), 3, 5); strings.Contains(bar, "running") {
		t.Errorf("a header with no spare room still drew the badge:\n%q", bar)
	}
}

// TestHeaderSaysNothingAboutMemoryItWasNotTold: before the runtime has answered, the board carries
// no figure, and a header inventing one would report a machine with nothing free.
func TestHeaderSaysNothingAboutMemoryItWasNotTold(t *testing.T) {
	labels := []string{"1 Repos", "2 Agents"}
	for _, repo := range []string{"", "ranke-db"} {
		if bar := headerBar(labels, 0, 120, repo, "rdb", 0, api.FleetMemory{}, 0, 0); strings.Contains(bar, "running") {
			t.Errorf("repo=%q: an unmeasured machine drew a badge:\n%q", repo, bar)
		}
	}
}

// plainHeaderWidth is what the tabs and the repo indicator take on their own — below it the bar
// overflows with or without a memory figure, which is a width the badge cannot be blamed for.
func plainHeaderWidth(labels []string, repoName string) int {
	w := 0
	for _, l := range labels {
		w += lipgloss.Width(l) + 3
	}
	if repoName != "" {
		w += lipgloss.Width("◉ "+repoName) + 1
	}
	return w
}
