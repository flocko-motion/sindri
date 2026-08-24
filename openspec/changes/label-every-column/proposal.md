# Every table says what its columns are

## Why

No table in the TUI names its columns, and neither did the CLI listings. A reader decodes each one
from the values — which works for a status word and fails for anything that looks like anything else.

Mail is where it bit: the recipient and the sender are adjacent, both agent-shaped names, and the
listing put `to` before `from`, the reverse of how mail is read anywhere else. Nothing in the row
corrects the reading, so it was misread over and over. Two unlabelled columns of the same shape are a
coin toss, and the coin was weighted the wrong way.

## What changes

- One label line above the rows in every tab and every CLI listing, dim, with no rule beneath it. A
  separator would cost a second line of list height in every tab and add nothing a blank-free dim line
  does not already say.
- `internal/ui/table` is the layout: a `Table` of `Column`s produces BOTH its `Header()` and each
  `Line()`. Every table therefore has exactly one set of widths, and a label sits over its column
  because the same arithmetic placed both. Each tab computed its own widths inline before, so a header
  written beside them would have been correct on the day and crooked after the first glyph change —
  `agent stats` already had a header of exactly that kind, and it is now laid out by the shared one.
- The label line is a ROW THAT SELECTS NOTHING, through the mechanism the foreign/local section
  headings already use (`row.selectable()`, `headingRow`, `moveCursor`). The two kinds of non-row are
  assembled by one function, `listing`, rather than each tab inventing its own — the second such line
  generalises the first rather than running beside it.
- Reading the selection is now `selRow()`, which snaps past any label. Storing and reading the cursor
  came apart because the arithmetic that MOVES it works in raw row indexes, while a model that has not
  been laid out yet has its cursor at 0 — which is where the labels are.
- Mail reads `from` then `to`, in both front-ends. Labelling a backwards order would only have made
  the backwardness legible.
- Where a header names the column, the rows stop repeating it: `repo list` printed "3 agents" and
  "issues:on" on every line, and now prints `3` and `on` under `agents` and `issues`. The PR wait
  reason likewise moves from a marker tacked onto the end of the line into a column of its own.

## What is deliberately NOT clipped

A column pads by default and only clips when it says so. Clipping every column would keep the table
square at the cost of half an id or half a status word — and an id you cannot copy is worse than a row
that sits wide. So `Clip` is set exactly where the old format strings had it (`%-10.10s`, the repo
name, whose values are unbounded) and nowhere else. `unapproved` in an eight-cell column still takes
the room it needs, exactly as before.

## Non-goals

Sticky labels. The line scrolls with the list, like the first line of any list. Freezing it would mean
every tab rendering its list in two pieces, which is a larger change than the confusion warrants.

The two-cell columns stay unlabelled: the task tree's gutter, and the marker column. Two characters
cannot name "an agent is on this, and it has a PR", and a cryptic label would be worse than the glyphs
it sat over — which the detail pane spells out.

## Impact

- Specs: `view-tui` gains the label line and the non-selectable rule beside its existing pane and
  navigation requirements; `view-workers` gains the CLI half and the mail order.
- Code: `internal/ui/table` (new), the six TUI tabs, `rowsections.go`, `items.go`, and six CLI
  listings plus `agent stats`.
- `internal/ui/tui/editor.go` is a lift, not a change: `tab_prs.go` went two lines over the
  700-line ceiling, and the editor/shell group is what was least about rendering a PR.
- Tests that indexed `rows[0]` as the first item, or counted rows to count items, now go through an
  `items` helper. The invariant is unchanged; a labelled list simply also carries the line naming its
  columns.
