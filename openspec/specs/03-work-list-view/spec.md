# Work List View

## Purpose

Defines the work list — the primary view of all work items — independent of any
interface. The CLI (`sindri task list`) and the TUI backlog are two renderings
of this one definition; any future UI renders the same.

## Requirements

### Requirement: Items as rows

The work list SHALL present each work item as a row showing its id, priority,
last-updated time, status, and title, in the order defined by the work-item
refresh. Spec-only items SHALL be marked as specs needing a task.

#### Scenario: Spec-only row

- **WHEN** a spec-only item is listed
- **THEN** its row is marked as a spec (e.g. "📋 spec") with its `os-……` id

### Requirement: Worker and orphan status

When a worker is assigned to an item, the row's status SHALL show the worker
instead of the raw status. An `in_progress` item with no worker SHALL be shown
as a warning (orphaned).

#### Scenario: Assigned worker

- **WHEN** a worker is working an item
- **THEN** the status cell shows that worker, not the bare "in_progress"

### Requirement: PRs and gates beneath their item

Each item's associated PRs and unmet/met review gates SHALL be shown beneath it,
visually subordinate to the item row.

#### Scenario: Item with a PR

- **WHEN** an item has an associated PR
- **THEN** the PR appears as a sub-row under the item with its status

### Requirement: Status filtering

The work list SHALL hide closed items by default and SHALL offer to show all
items, only open items, or only closed items.

#### Scenario: Default view

- **WHEN** the work list is shown with no filter
- **THEN** closed, approved, and merged items are omitted

### Requirement: Identical across interfaces

Every interface that renders the work list SHALL show the same items with the
same fields, differing only in presentation (table vs. scrolling viewport).

#### Scenario: CLI and TUI agree

- **WHEN** the CLI list and the TUI backlog are shown for the same project
- **THEN** they contain the same items with the same per-row fields
