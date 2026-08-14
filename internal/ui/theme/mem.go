// package: ui/theme / mem
// type:    rendering (shared presentation primitives)
// job:     render an agent's memory use against its limit as one line, and the fleet's headroom as
// a line or a header badge, so the CLI and the TUI show the same figures in the same
// shape.
// limits:  formatting only; the numbers come from the hub, which reads them off the runtime.
package theme

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/flo-at/sindri/internal/api"
)

// MemLine renders "544 MiB / 1024 MiB  53% [█████·····]" for a usage/limit pair.
func MemLine(usage, limit int64) string {
	pct := 0.0
	if limit > 0 {
		pct = float64(usage) / float64(limit) * 100
	}
	return fmt.Sprintf("%9s / %-9s %3.0f%% %s", humanBytes(usage), humanBytes(limit), pct, memBar(pct))
}

// FleetLine renders the fleet's headroom in full: what the machine's agents cost it against the
// ceiling, then the two figures worth reading — what is free, and how many more agents fit there.
func FleetLine(m api.FleetMemory) string {
	if !m.Known() {
		return "not reported by the runtime"
	}
	pct := float64(m.UsedBytes) / float64(m.TotalBytes) * 100
	return fmt.Sprintf("%s / %s %s  %3.0f%% %s   %s free · %s",
		humanBytes(m.UsedBytes), humanBytes(m.TotalBytes), m.Basis, pct, memBar(pct),
		humanBytes(m.FreeBytes()), fitsPhrase(m))
}

// fitsPhrase says how many more default-size agents fit, naming the size counted, so the number
// can be read without knowing what the default is.
func fitsPhrase(m api.FleetMemory) string {
	if m.AgentBytes <= 0 {
		return "the default agent size is unknown"
	}
	switch m.Fits {
	case 0:
		return fmt.Sprintf("no room for another agent (%s each)", humanBytes(m.AgentBytes))
	case 1:
		return fmt.Sprintf("room for 1 more agent (%s each)", humanBytes(m.AgentBytes))
	}
	return fmt.Sprintf("room for %d more agents (%s each)", m.Fits, humanBytes(m.AgentBytes))
}

// FleetBadge renders the same headroom for a header, in the widest form that fits `width` cells,
// and "" when even the shortest does not — a header owes its space to the tabs first. The forms
// shed the meter, then the free figure, keeping the fit count longest: it is the answer, and the
// rest is the working.
func FleetBadge(m api.FleetMemory, width int) string {
	if !m.Known() {
		return ""
	}
	pct := float64(m.UsedBytes) / float64(m.TotalBytes) * 100
	free := humanBytes(m.FreeBytes())
	for _, form := range []string{
		fmt.Sprintf("%s %s free · fits %d agents", memBar(pct), free, m.Fits),
		fmt.Sprintf("%s free · fits %d", free, m.Fits),
		fmt.Sprintf("fits %d", m.Fits),
	} {
		if lipgloss.Width(form) <= width {
			return form
		}
	}
	return ""
}

// humanBytes uses binary units, matching how memory limits are configured.
func humanBytes(n int64) string {
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	f, units, i := float64(n), []string{"KiB", "MiB", "GiB", "TiB"}, -1
	for f >= u && i < len(units)-1 {
		f, i = f/u, i+1
	}
	return fmt.Sprintf("%.0f %s", f, units[i])
}

// memBar is a 10-cell usage meter; fuller = closer to the limit.
func memBar(pct float64) string {
	const w = 10
	fill := int(pct/100*w + 0.5)
	if fill > w {
		fill = w
	}
	if fill < 0 {
		fill = 0
	}
	return "[" + strings.Repeat("█", fill) + strings.Repeat("·", w-fill) + "]"
}
