# The PRs detail column can be scrolled

## Why

The report was that the PRs tab detail does not scroll. Two earlier readings looked at the diff
pane and at the viewport's dimensions. Neither was the fault.

The diff pane scrolls correctly, and now has tests saying so: `J`/`K` move it, `ctrl+d`/`ctrl+u`
half-page the same content by more than `J` does, and the position survives a board poll.

The defect is the RIGHT column. `prBody` built its viewport as a local variable on every render:

    var rv scroll.Viewport
    rv.Resize(h, len(lines))
    right := pane(lines, rv, rightW, hl)

Nothing outside that function could reach it, so its offset was permanently zero. That column
carries the rejection feedback, every review with its findings, and the whole history — and all of
it below the first screenful was unreachable by any key. The Agents tab renders its right column
through `m.detail` and scrolls fine; the PRs tab spends `m.detail` on the diff and had nothing
left for the column.

The cursor could not rescue it either: the column's cursor stops only on actionable items, which
all sit in the first dozen lines, and the reviews and history are plain text.

## What changes

- The right column gets its own viewport on the model, `prMeta`, sized in `reclamp`.
- On the PRs tab, `J`/`K` follow the focus: the diff from the list, the column once focused. This
  supersedes sd-ac8831's "not focus-relative" for this tab only — that decision was made for tabs
  with ONE detail pane, and here two scrollable regions share one pair of keys. Every other tab is
  unaffected, since there the focused pane and the detail pane are the same thing.
- `ctrl+d`/`ctrl+u` keep half-paging the diff, unchanged.
- The column is offset-driven like the diff, deliberately not cursor-following: re-following a
  cursor on every board refresh would drag the view back on each tick, and a scroll undone before
  the eye registers it is indistinguishable from one that never happened.

## Impact

- Specs: `view-tui` gains what the scroll keys drive; the panes were specified as scrollable but
  never which key reaches which.
- Code: `internal/ui/tui/tui.go` (the field), `viewport.go` (`scrollTarget`, sizing),
  `tab_prs.go` (`prMetaLines`, rendering through the model's viewport), `onkey.go` (`J`/`K`).
- No new key and no new footer entry: the `J/K — scroll detail` line sd-ac8831 added is now true
  on this tab rather than aspirational.
