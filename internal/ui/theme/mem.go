// package: ui/theme / mem
// type:    rendering (shared presentation primitives)
// job:     render an agent's memory use against its limit as one line, so the CLI's stats table and
// the TUI's stats view show the same figure in the same shape.
// limits:  formatting only; the numbers come from the hub.
package theme

import (
	"fmt"
	"strings"
)

// MemLine renders "544 MiB / 1024 MiB  53% [█████·····]" for a usage/limit pair.
func MemLine(usage, limit int64) string {
	pct := 0.0
	if limit > 0 {
		pct = float64(usage) / float64(limit) * 100
	}
	return fmt.Sprintf("%9s / %-9s %3.0f%% %s", humanBytes(usage), humanBytes(limit), pct, memBar(pct))
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
