# view-tui — delta

## MODIFIED Requirements

### Requirement: vi navigation

The TUI SHALL navigate vi-style: `ctrl+h`/`ctrl+l` switch tabs (and `1`/`2`/`3`
jump to one); `j`/`k` move the selection, `g`/`G` jump to top/bottom; in the task
tree `h`/`l` collapse/expand. Moving the selection SHALL update the detail pane
immediately (no separate open step). `J`/`K` SHALL scroll the detail pane
directly, from either pane and with no separate step to reach it — the
yazi-style secondary-pane scroll a terminal user already has in their fingers.

Where a view has a SECOND scrollable region beside its detail pane, `J`/`K` SHALL
drive whichever of the two holds the focus, so that both are reachable without
spending a further binding. A view with a single scrollable region is unaffected:
there the focused pane and the detail pane are the same thing, and the keys reach
it from either side exactly as before.

#### Scenario: Tab switch

- **WHEN** the user presses `ctrl+l`
- **THEN** the tab switches forward and the pane focus resets

#### Scenario: Detail pane scrolls regardless of focus

- **WHEN** the user presses `J` or `K` in a view with one scrollable region,
  whether or not the detail pane currently has focus
- **THEN** the detail pane scrolls down or up; the list selection and its own
  pane are unaffected

#### Scenario: The second region takes the keys while it holds the focus

- **WHEN** the user focuses the PRs tab's detail column and presses `J` or `K`
- **THEN** the column scrolls, bringing its reviews, findings and history into
  view, and the diff pane does not move

#### Scenario: The detail pane keeps the keys from the list

- **WHEN** the user presses `J` or `K` with the PR list focused
- **THEN** the diff pane scrolls and the detail column does not move

## ADDED Requirements

### Requirement: A scrolled pane holds its position across a refresh

Where the user has scrolled a pane, a board refresh SHALL NOT return it to the top. The board
updates on its own schedule, so a position surrendered on the next update cannot be read: the
content moves, then leaves before the eye reaches it, which is indistinguishable from a pane that
never scrolled at all.

#### Scenario: A refresh arrives after scrolling

- **WHEN** a board refresh arrives after the user has scrolled a pane
- **THEN** the pane still shows the position it was scrolled to
