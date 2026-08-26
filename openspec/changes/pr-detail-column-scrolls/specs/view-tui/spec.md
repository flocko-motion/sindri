# view-tui — delta

## MODIFIED Requirements

### Requirement: vi navigation

The TUI SHALL navigate vi-style: `tab`/`shift+tab` switch tabs (and `1`/`2`/`3`
jump to one); `j`/`k` move the selection, `g`/`G` jump to top/bottom; in the task
tree `h`/`l` collapse/expand. Moving the selection SHALL update the detail pane
immediately (no separate open step). `ctrl+l`/`ctrl+h` focus the detail pane and
return from it; focused, `j`/`k` scroll it one line at a time instead, and `g`/`G`
jump it to top/bottom.

Where a view has a SECOND scrollable region beside its detail pane, `ctrl+l`
SHALL step through both in turn (list, then the detail pane, then the second
region), and `j`/`k`/`g`/`G` SHALL drive whichever one currently holds the
focus — so both are reachable without a further binding. A view with a single
scrollable region is unaffected: there the focused pane and the detail pane
are the same thing.

#### Scenario: Tab switch

- **WHEN** the user presses `tab`
- **THEN** the next tab becomes active

#### Scenario: Selection drives detail

- **WHEN** the user moves the selection with `j`/`k`
- **THEN** the detail pane shows the newly selected item

#### Scenario: A focused pane scrolls with j/k

- **WHEN** the user focuses a view's detail pane (`ctrl+l`) and presses `j`
  or `k`
- **THEN** the pane scrolls down or up one line; the list selection is
  unaffected

#### Scenario: The second region takes the keys while it holds the focus

- **WHEN** the user steps `ctrl+l` again to focus the PRs tab's metadata
  column and presses `j` or `k`
- **THEN** the column steps to the next/previous item there, scrolling once
  past the last one to reach its reviews, findings and history — and the
  diff pane does not move

#### Scenario: The list keeps j/k until the diff is focused

- **WHEN** the user presses `j` or `k` with the PR list focused
- **THEN** the selection moves and the diff pane does not scroll — reaching
  it takes focusing it first (`ctrl+l`)

## ADDED Requirements

### Requirement: A scrolled pane holds its position across a refresh

Where the user has scrolled a pane, a board refresh SHALL NOT return it to the top. The board
updates on its own schedule, so a position surrendered on the next update cannot be read: the
content moves, then leaves before the eye reaches it, which is indistinguishable from a pane that
never scrolled at all.

#### Scenario: A refresh arrives after scrolling

- **WHEN** a board refresh arrives after the user has scrolled a pane
- **THEN** the pane still shows the position it was scrolled to
