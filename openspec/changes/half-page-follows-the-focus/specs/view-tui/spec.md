# view-tui — delta

## ADDED Requirements

### Requirement: The half-page keys follow the pane focus

`ctrl+d`/`ctrl+u` SHALL move whichever column holds the focus, being the coarse form of the keys
that move it a line at a time. With the list focused they SHALL move the selection by half the body
height, which is how a list scrolls — it has no viewport of its own, and the selected line stays in
view. With the detail column focused they SHALL scroll that column by half its height, resolving
which region that is exactly as `j`/`k` do there, so both speeds reach the same content.

They SHALL NOT decide from the active tab. A tab is not what the user is looking at within it, and
deciding that way sent the keys to the list while the detail column had the focus.

#### Scenario: The list is focused

- **WHEN** the user presses `ctrl+d` with the list focused
- **THEN** the selection moves down by half the body height and no pane scrolls under it

#### Scenario: The detail column is focused

- **WHEN** the user presses `ctrl+d` with the detail column focused
- **THEN** that column scrolls down by half its height and the selection stays where it was

#### Scenario: A view with two scrollable regions

- **WHEN** the user focuses the PRs tab's metadata column and presses `ctrl+d`
- **THEN** the column half-pages, as `j`/`k` would move it there, and the diff pane does not move
