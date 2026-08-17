# view-tui — delta

## MODIFIED Requirements

### Requirement: Full-terminal tabbed master-detail layout

The TUI SHALL fill the whole terminal at any size: a tab strip on the top row, a
left selector column and a right detail pane below it, and a footer pinned to the
last two rows. The layout SHALL NOT collapse or leave dead space when there are
few items — the panes are sized from the terminal height so the footer is always
on the last row.

The rendered frame SHALL be EXACTLY the terminal's height, and no line of it wider than the
terminal's width. A frame one row too tall makes the terminal scroll, and what scrolls away is the
top row: the tab strip, the repo name and the memory headroom, leaving whatever the tab draws first
where the header belongs. Filling the terminal and not exceeding it are one contract, and the second
half is the half that is easy to lose.

Every tab SHALL honour that contract, including one that composes its own body rather than laying it
out through the shared pane primitive. A tab that reserves rows for content of its own — a permanent
note above its panes — SHALL take those rows out of the panes beneath it, so its viewports are sized
to the slots they occupy rather than to the whole body.

#### Scenario: Few items still fills the frame

- **WHEN** a tab has only one or two items
- **THEN** the selector/detail panes still extend to full height and the footer
  remains on the last two rows

#### Scenario: Resize

- **WHEN** the terminal is resized
- **THEN** the panes and footer re-flow to the new size, footer still last

#### Scenario: A tab with a reserved row keeps the header

- **WHEN** a tab draws its own content above its panes and is rendered at any terminal size
- **THEN** the frame is exactly the terminal's height and the tab strip is still the top row
