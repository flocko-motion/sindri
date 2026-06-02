## ADDED Requirements

### Requirement: Loading state distinct from empty

Before the first board refresh has applied, the work list SHALL show a
"Loading tasks…" placeholder, distinct from the empty-state placeholder used
when a refresh has applied but no items match the current filter. The loading
placeholder SHALL replace any empty-state text in this window so the user is
never misled into thinking the board is empty before the data has arrived.

#### Scenario: First frame after startup

- **WHEN** the TUI is rendered after the window has sized itself but before
  the first refresh has applied
- **THEN** the work-list panel shows "Loading tasks…" instead of
  "No tasks or PRs"

#### Scenario: Refresh applied, board truly empty

- **WHEN** a refresh has applied and the filtered list contains no items
- **THEN** the work-list panel shows its empty-state placeholder ("No tasks
  or PRs"), not the loading text
