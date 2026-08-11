# view-tui — delta

## ADDED Requirements

### Requirement: Every scrollable region is reachable by a key

Where a view shows content taller than its pane, some key SHALL scroll it. A pane whose content
cannot be reached is a pane that has hidden it.

The detail scroll keys SHALL drive the view's detail pane from either pane. Where a view has more
than one scrollable region and only one pair of scroll keys, those keys SHALL drive whichever
region holds the focus, so that each region is reachable without spending a new binding.

A region scrolled this way SHALL keep its position across a board refresh, since a view that
returns to the top on the next update cannot be read.

#### Scenario: The PR detail column is scrolled

- **WHEN** the user focuses the PRs tab's detail column and presses the scroll keys
- **THEN** the column scrolls, bringing its reviews, findings and history into view, and the diff
  pane does not move

#### Scenario: The diff is scrolled from the list

- **WHEN** the user presses the scroll keys with the PR list focused
- **THEN** the diff pane scrolls and the detail column does not move

#### Scenario: A scrolled pane survives a refresh

- **WHEN** a board refresh arrives after the user has scrolled a pane
- **THEN** the pane holds the position it was scrolled to
