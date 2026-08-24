# Tasks

## 1. One layout, both jobs

- [x] 1.1 `internal/ui/table`: a `Table` of `Column`s produces its `Header()` and each `Line()` from
      the same widths, so a label sits over its column because the same arithmetic placed both.
- [x] 1.2 A `Cell` carries its own style, applied AFTER padding — a style applied first has its escape
      sequences counted as content, and every column after it sits early.
- [x] 1.3 Widths are DISPLAY cells, not runes: the marker column is padded from a glyph's drawn width.
- [x] 1.4 `Clip` is opt-in, set only where the old format strings clipped (`%-10.10s`, a repo name).
      Elsewhere a column pads and an over-long value takes the room it needs — half an id cannot be
      copied and half a status word cannot be read.

## 2. Label every tab

- [x] 2.1 A table per tab: tasks, agents, PRs, repos, runs, mail. Each tab's rows go through it, so the
      inline `Sprintf` widths are gone and there is one set left.
- [x] 2.2 `listing` assembles a tab's whole row list — the labels, then the rows, sectioned where some
      are foreign. One assembler, so no tab can forget its labels.
- [x] 2.3 The label line is a row that selects nothing, the mechanism the section headings already use
      rather than a second one beside it.
- [x] 2.4 `selRow` reads the selection past any label, so a model that has not been laid out yet still
      selects a row rather than the line naming the columns.
- [x] 2.5 Nothing is labelled where there are no rows; each tab's own empty state stands.

## 3. Fix the mail order

- [x] 3.1 `from` before `to`, in both front-ends.

## 4. Label every CLI listing

- [x] 4.1 `agent list`, `pr list`, `task list`, `mail list`, `run list`, `repo list` — each through its
      own table, printed under `printListing` / `printRows`.
- [x] 4.2 `agent stats` had a header already, from widths of its own beside the rows'. It reads the
      shared layout now, which is the whole point of having one.
- [x] 4.3 Rows stop repeating what the header says: `repo list`'s "3 agents"/"issues:on" become values
      under labels, and a PR's wait reason moves into a column instead of trailing the line.

## 5. Pin it

- [x] 5.1 The layout itself: labels over their columns, right-aligned columns ending where their
      labels do, the trailing column unpadded, styling not disturbing the widths, display-cell widths,
      and clip-versus-pad both ways.
- [x] 5.2 Every tab with rows opens with a line that is not selectable AND equals its own table's
      header — so a tab that lays out its own labels fails rather than looking fine alone.
- [x] 5.3 Every label fits its column, and only the last column is unbounded, over every table in both
      front-ends.
- [x] 5.4 The cursor cannot land on the labels, on every tab, by every key that moves it.
- [x] 5.5 No labels without rows, over every tab.
- [x] 5.6 Sender before recipient, in both front-ends.

## 6. Keep the files honest

- [x] 6.1 `tab_prs.go` went two lines over the 700-line ceiling; the editor/shell group — the part
      least about rendering a PR — lifts out to `editor.go` with its own header.
