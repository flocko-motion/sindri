# view-tui — delta

## MODIFIED Requirements

### Requirement: Tasks filter toggle

The Tasks tab SHALL provide a filter, cycled with `f`, over the four filters the
exchange package defines: open → closed → all → active, in that order, defaulting to
active. What each admits SHALL come from that shared definition rather than from the
TUI, so the tab and `sindri task list --filter` show the same tasks. The active filter
SHALL be shown in the footer and applied to the displayed task tree. The tab's badge
count SHALL remain the non-closed count regardless of the active filter.

The tab opens on active — the open backlog plus whatever has changed recently — because
plain open hid a task the moment it closed, so the work just finished left no trace and
the tab read as though nothing had happened.

#### Scenario: Toggle to closed

- **WHEN** the user presses `f` until the filter is "closed"
- **THEN** the tree shows only done tasks, while the tab badge still counts
  non-closed tasks

#### Scenario: Default is active

- **WHEN** the Tasks tab is first shown
- **THEN** it lists not-done tasks together with any task that changed inside the
  active window

#### Scenario: The cycle covers every filter

- **WHEN** the user presses `f` four times from any filter
- **THEN** each of the four has been shown once and the tab is back where it started
