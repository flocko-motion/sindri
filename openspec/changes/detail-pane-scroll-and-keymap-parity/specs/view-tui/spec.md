# View: TUI Dashboard — delta

## MODIFIED Requirements

### Requirement: Panes are fixed-height scrollable regions

Every content region — the selector and the detail pane — SHALL be a fixed-height
pane that displays content of any length: content shorter than the pane is padded
to fill it, content longer than the pane scrolls. A pane SHALL always render
exactly its assigned height. All such regions SHALL use one shared scroll
primitive rather than per-pane offset logic. Content with no actionable item of
its own — a comment thread, or anything else the selector's cursor cannot land
on — SHALL still be reachable by scrolling: the cursor's reach and the pane's
scroll extent are independent, and a pane SHALL NOT bound the latter by the
former.

#### Scenario: Content shorter than the pane

- **WHEN** a pane's content is shorter than its height
- **THEN** it is padded to full height (no scrolling), and the layout around it is
  unaffected

#### Scenario: Content longer than the pane

- **WHEN** a pane's content exceeds its height
- **THEN** it scrolls within its fixed height, and the selected/focused line stays
  in view

#### Scenario: Content past the last actionable item is reachable by scrolling

- **WHEN** a detail pane's content continues past its last cursor-reachable
  item — a task's comment thread, rendered after its cross-references
- **THEN** scrolling the pane still reaches that content, regardless of
  whether anything in it can be selected

### Requirement: vi navigation

The TUI SHALL navigate vi-style: `ctrl+h`/`ctrl+l` switch tabs (and `1`/`2`/`3`
jump to one); `j`/`k` move the selection, `g`/`G` jump to top/bottom; in the task
tree `h`/`l` collapse/expand. Moving the selection SHALL update the detail pane
immediately (no separate open step). `J`/`K` SHALL scroll the detail pane
directly — unconditionally, from either pane, never gated by which pane
currently has focus — the yazi-style secondary-pane scroll a terminal user
already has in their fingers.

#### Scenario: Tab switch

- **WHEN** the user presses `ctrl+l`
- **THEN** the tab switches forward and the pane focus resets

#### Scenario: Detail pane scrolls regardless of focus

- **WHEN** the user presses `J` or `K`, whether or not the detail pane
  currently has focus
- **THEN** the detail pane scrolls down or up; the list selection and its own
  pane are unaffected
