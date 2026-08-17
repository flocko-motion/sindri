# Task row markers: glyphs, names and column widths

## Why

The markers in the task and agent rows were emoji written as literals in the view that drew them.
Both halves of that are a problem.

An emoji's drawn width is the classic source of terminal disagreement — which is why the marker
column is padded to a fixed number in the first place, and why the eye and the warning carry a
variation selector to pin them at two cells. A Nerd Font icon lives in the Private Use Area, where
a width counter and a terminal cannot disagree: neither has an East Asian Width to consult, so both
count one cell whether or not the glyph exists. The switch removes the hazard rather than trading it.

Written as literals, the same symbol drifted: the CLI's eye and the TUI's differed by a variation
selector, and nothing held them together. `internal/ui/theme` is the shared rendering module both
front-ends already read, and the markers belong there.

## What changes

- One marker set in `internal/ui/theme/glyph.go`: worked-on, PR (final and interim), dial-in,
  warning, retired, clear-armed, needs-user. Both front-ends read it; neither writes a symbol.
- The pictorial markers become Nerd Font icons from the Font Awesome block every Nerd Font has
  carried unchanged. The attention mark stays ASCII — it sits inside the header strip and in a CLI
  line, where a word's worth of alarm reads better than a picture — and the clear mark stays a plain
  symbol, legible in any font.
- The marker column's width is measured from the marks rather than written down, since the count
  just changed under it: the hammer was two cells and the wrench is one.
- A terminal without a patched font shows boxes in one narrow column and is otherwise identical.
  That is the accepted outcome: there is no way to ask a terminal whether it has a glyph, so a
  detected fallback would be a guess, and the width agreement is what makes the guess unnecessary.

## Impact

- Specs: `view-tui` (one shared marker set, one cell each, widths derived).
- Code: `internal/ui/theme/glyph.go` (new), `internal/ui/tui/tab_tasks.go`,
  `internal/ui/tui/tab_agents.go`, `internal/ui/tui/tab_prs.go`, `internal/ui/cli/agent.go`,
  `internal/ui/cli/hub.go`.
- The PR pair stays ◆/◇ here: the shapes carry the final-versus-interim distinction, and the
  pull-request icon that replaces them belongs to the sibling task working the same column.
- `TestGlyphsCountAsTerminalsDrawThem` asserted the old two-cell emoji and now holds the new
  invariant — one cell per marker, which is the property the icons were chosen for.
