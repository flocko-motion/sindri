# Detail pane: content past the last actionable item is reachable, and its keys are advertised

## Why

Detail content that sits below the last ACTIONABLE item could not be reached. A
task's comment thread is the case that exposed it: `commentItems` renders plain
lines with no `kind`, so `rightFocus`'s cursor (which only walks
`actionableItems()`) can never land on one, and — because `J`/`K` (the
yazi-style secondary-pane scroll already implemented in `onKey`) were absent
from the keymap table — nobody discovered that scrolling, not selecting,
reaches it. `g`/`G` (top/bottom) and, on the task tree, `h`/`l`
(collapse/expand) were in the same state: implemented, already specified by
`view-tui`'s "vi navigation" requirement, yet absent from the keymap table's
own footers, so a binding the spec requires still shipped invisible.

Diagnosed against the running UI (via the `Screenshot` harness, not by trusting
the source's own comments): `J`/`K` already scroll the detail pane
unconditionally, from either pane — `rightFocus` never gates them. The reported
"J/K move the main pane" experience is best explained by the missing keymap
entry itself: a user reaching for vi muscle memory tries lowercase `j`/`k`
first, which (correctly, by design) moves the list selection while unfocused,
with no footer hint that Shift is what reaches the detail pane instead.

## What Changes

- `keys.go`'s keymap gains rows for `J`/`K` (scroll detail), `g`/`G`
  (top/bottom), `y`/`Y` (yank/yank all), `ctrl+d`/`ctrl+u` (page), and, task-tab
  only, `h`/`l` (fold) — closing the gap between what `onKey` implements and
  what the footers advertise, budgeted deliberately (grouped, terse labels)
  rather than appended one row per key.
- A fix in `keys.go`'s existing `C-h/l` display row: written that way it
  silently also registered plain `l` (a real, different binding — task-tree
  expand) under the "pane" label. Corrected to `C-h/C-l`.
- A new test, `TestEveryHandledLetterKeyIsInTheKeymap`, parses `onkey.go`'s
  switch statements and `keys.go`'s key constants via `go/ast`, and fails the
  build if a single-letter key `onKey` dispatches on has no keymap row — the
  inverse of the existing `TestNoTwoActionsShareAKeyOnATab`, so this class of
  gap cannot reopen unnoticed.
- A new regression test proves the pinned behaviour directly against a real
  comment thread: `J` reaches content past the last actionable item, from
  either pane (focused or not).

## Capabilities

### Modified Capabilities

- `view-tui`: "vi navigation" gains `J`/`K` as the always-available secondary-
  pane scroll (not focus-relative); "Panes are fixed-height scrollable
  regions" states explicitly that scrolling, not the cursor, is what reaches
  content with no actionable item of its own.

## Impact

- **Source of truth:** `internal/ui/tui/keys.go` (keymap rows),
  `internal/ui/tui/keyparity_test.go` (the guarantee),
  `internal/ui/tui/detailscroll_test.go` (the regression test).
- No behaviour change to `onKey` itself — `J`/`K`/`g`/`G`/`y`/`Y`/`h`/`l` already
  worked; this makes them discoverable and keeps them that way.
