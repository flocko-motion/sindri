# view-tui — delta

## ADDED Requirements

### Requirement: Every table names its columns

Each tab that renders a table SHALL show a line of column labels above its rows, styled as chrome so
it never reads as a row. A reader decodes an unlabelled column from its values, which works for a
status word and fails for anything that looks like anything else — two adjacent columns of the same
shape are a guess.

The labels SHALL be laid out by the SAME column widths as the rows, from one layout that produces
both. A header assembled from widths of its own is aligned on the day it is written and crooked after
the first glyph or field change, and a crooked header is worse than none.

No separator rule SHALL be drawn beneath the labels. A rule costs a second line of list height in
every tab and says nothing the dim label line has not already said.

The label line SHALL NOT be selectable, and SHALL use the same mechanism as the section headings a
scoped list carries: a list has ONE kind of line that is not a row. The cursor walks the row list, so
a label it could rest on is a selection with nothing behind it and a detail pane with nothing to show.

No labels SHALL be shown where there are no rows. Labels over an empty table explain a table that is
not there, and each tab's own empty state says more.

A column two cells wide MAY go unlabelled — the task tree's gutter, the row-marker column. Two
characters cannot name what those carry, and a cryptic label would be worse than the glyphs it sat
over.

#### Scenario: Labels over the rows

- **WHEN** any tab with a table is shown with rows in it
- **THEN** its first line names its columns, dimmed, with each label over the column it names

#### Scenario: The cursor steps over the labels

- **WHEN** the user moves the selection to the top of the list with `g` or `k`
- **THEN** the cursor rests on the first row, and the detail pane shows it

#### Scenario: An empty table is unlabelled

- **WHEN** a tab has no rows to show
- **THEN** no label line is drawn, and its own empty state is what appears

### Requirement: Mail is listed sender before recipient

The mail list SHALL show the sender before the recipient. Both are agent-shaped names in adjacent
columns, so nothing in the row corrects a misreading, and the reverse order is the reverse of how mail
is read everywhere else. Labelling a backwards order would only make the backwardness legible.

#### Scenario: Reading a message's row

- **WHEN** the Mail tab lists a message
- **THEN** its `from` column precedes its `to` column
