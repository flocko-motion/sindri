# The frame never outgrows the terminal, and a test says so

## Why

The Runs tab renders a frame two lines taller than the terminal, so the terminal scrolls and the top
row — the tab strip, the repo name, the memory badge — goes with it. What is left where the header
belongs is the tab's permanent note, which reads as the note having replaced the header.

`reclamp` names tab 5 in two arms of one `else if` chain. The first match wins, so the Runs-specific
resize was unreachable and the detail viewport was sized to `bodyHeight()` rather than to
`runsPaneHeight()`. `runsBody` then joins a list sized to its slot against a detail pane padded to the
full body; `JoinHorizontal` takes the taller, so the body came out at full height and the note on top
of it overflowed by exactly the note's own height.

Nothing checked the height contract. Every tab honours it by its own arithmetic, and Runs is the only
one that composes a body by hand rather than through `pane`, which is why it is the one that broke.

## What changes

- The detail pane's slot is chosen where the list's already is, at the top of `reclamp`: the body
  height everywhere, `runsPaneHeight()` on Runs. The unreachable arm goes.
- A test asserts the rendered frame is exactly the terminal's height, and no line wider than its
  width, over EVERY tab, at three terminal sizes, with the detail column shown and hidden. It fails
  before the fix — 26 lines in a 24-row terminal — and is what stops the next hand-composed body
  doing the same.

## The dead arm was also wrong, so it is not resurrected

The unreachable code sized the viewport to `len(m.detailLines())`, the UNWRAPPED line count, while
`runsBody` renders `wrappedDetail()`. Making that arm reachable as written fixes the height and
breaks the scroll: with a run's stored output the viewport counted 12 lines where the pane renders 66,
so five-sixths of the output would be unreachable by `J`/`K`. Runs keeps the wrapped count the generic
arm already gave it; only the height was ever wrong. A second test pins the count, and it is the one
that fails under the literal fix.

## Impact

- Specs: `view-tui`'s layout requirement says the TUI fills the terminal. It gains the other half —
  never MORE than the terminal — and says that a tab composing its own body owes the same contract.
  That half was assumed by every tab and stated by none, which is how it went unenforced.
- Code: `internal/ui/tui/viewport.go` only.
- Runs' `J`/`K` reaches the end of a long output, having previously believed the pane had more room
  below than the screen gives.
