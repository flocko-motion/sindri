## ADDED Requirements

### Requirement: Loading state distinct from empty

Before the first board refresh has applied, the workers view SHALL show a
"Loading workers…" placeholder, distinct from the empty-state placeholder used
when a refresh has applied but no workers exist. The loading placeholder SHALL
replace any empty-state text in this window so the user is never misled into
thinking there are no workers before the data has arrived.

#### Scenario: First frame after startup

- **WHEN** the TUI is rendered after the window has sized itself but before
  the first refresh has applied
- **THEN** the workers panel shows "Loading workers…" instead of "No workers"

#### Scenario: Refresh applied, no workers

- **WHEN** a refresh has applied and the project has no workers
- **THEN** the workers panel shows its empty-state placeholder ("No workers")
