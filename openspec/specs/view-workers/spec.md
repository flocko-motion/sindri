# View: Workers

## Purpose

Defines the workers view — the list of sindri workers and what each is doing —
independent of any interface. `sindri worker list` and the TUI workers view are
renderings of this one definition.
## Requirements
### Requirement: Worker rows

The workers view SHALL list every worker with its name, role (worker or
reviewer), running status, current task, associated PR, workspace path, and
current branch.

#### Scenario: Running worker

- **WHEN** a worker is running on a task
- **THEN** its row shows the task and the branch it is on

### Requirement: Stopped workers visible

Workers that have a worktree but no running container SHALL still be listed,
shown as not running, so idle/stopped workers are visible rather than hidden.

#### Scenario: Stopped worker

- **WHEN** a worker's container is not running but its worktree exists
- **THEN** it appears in the list marked as not running

### Requirement: Reviewer distinct

The review agent SHALL appear distinctly from the dwarf workers (a separate
role), not as one of them.

#### Scenario: Listing with a reviewer

- **WHEN** the reviewer and dwarf workers are listed together
- **THEN** the reviewer is shown with the reviewer role, not as a dwarf worker

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

