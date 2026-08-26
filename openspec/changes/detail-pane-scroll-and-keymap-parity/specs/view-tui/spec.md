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

The TUI SHALL navigate vi-style: `tab`/`shift+tab` switch tabs (and `1`/`2`/`3`
jump to one); `j`/`k` move the selection, `g`/`G` jump to top/bottom; in the task
tree `h`/`l` collapse/expand. Moving the selection SHALL update the detail pane
immediately (no separate open step). `ctrl+l`/`ctrl+h` focus the detail pane and
return from it; focused, `j`/`k` scroll it one line at a time instead, and `g`/`G`
jump it to top/bottom — reaching content with no actionable item of its own,
since the cursor never lands on it.

#### Scenario: Tab switch

- **WHEN** the user presses `tab`
- **THEN** the next tab becomes active

#### Scenario: Selection drives detail

- **WHEN** the user moves the selection with `j`/`k`
- **THEN** the detail pane shows the newly selected item

#### Scenario: A focused detail pane scrolls with j/k

- **WHEN** the user focuses the detail pane (`ctrl+l`) and presses `j` or `k`
- **THEN** the pane scrolls down or up one line; the list selection and its
  own pane are unaffected

## ADDED Requirements

### Requirement: Jump to the next/previous row needing the user

`]`/`[` SHALL move the selection to the next/previous VISIBLE row that needs
the user, on a tab whose rows can need it (Tasks, Agents, PRs, Mail) — read
off the same predicate that already marks that row (each also feeding its
section's attention badge, except Mail's own to-you-and-unread marker, whose
badge parity is a separate, later fix). They SHALL NOT cycle: past the last
match they SHALL leave the selection and say so rather than wrapping or doing
nothing silently. A row hidden by the active filter or folded under a
collapsed parent SHALL NOT be a candidate. On a tab with no such notion,
`]`/`[` SHALL do nothing and SHALL NOT be advertised.

#### Scenario: Jump to the next row needing the user

- **WHEN** the user presses `]` on a tab with rows that can need the user
- **THEN** the selection moves to the next visible such row, and the detail
  pane updates for it

#### Scenario: No further match

- **WHEN** the user presses `]` and no later visible row needs the user
- **THEN** the selection does not move, and the TUI says so rather than doing
  nothing silently

#### Scenario: A tab with no such notion

- **WHEN** the user presses `]`/`[` on a tab with no rows that can need the user
- **THEN** nothing happens, and the tab does not advertise the keys
