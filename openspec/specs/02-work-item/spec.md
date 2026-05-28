# Work Item Model

## Purpose

Defines the work item — the single state every interface renders — and the one
path that produces it. A work item unifies a td task and an openspec change so
that all views and actions operate on one shape, independent of any UI.

## Requirements

### Requirement: The work item union

A work item SHALL be the union of an optional task and an optional spec, in one
of three shapes: a task with no spec, a task implementing a spec, or a spec with
no task yet. At least one of the two SHALL be present.

#### Scenario: Spec with no task

- **WHEN** an openspec change has no task linked to it
- **THEN** it appears as a spec-only work item (needing a task)

#### Scenario: Task linked to a spec

- **WHEN** a task carries a `spec:<name>` label
- **THEN** its work item also carries that spec

### Requirement: Stable identity

Every work item SHALL expose a stable id: the task id (`td-……`) for an item with
a task, or a derived `os-……` id for a spec-only item.

#### Scenario: Spec-only id

- **WHEN** a spec-only item is shown
- **THEN** its id is a stable `os-……` derived from the spec name

### Requirement: Review gates

A work item SHALL derive its review gates from the task's labels: each
`require-review-<x>` is a gate, satisfied when a matching `approved-review-<x>`
is present. The set of unmet gates SHALL be queryable.

#### Scenario: Unmet gate

- **WHEN** a task has `require-review-code` but not `approved-review-code`
- **THEN** the item reports one unmet gate

### Requirement: Single refresh path

The complete work-item state SHALL be produced by one refresh function that
gathers tasks, specs, worker assignments, and PRs and assembles them. Every
interface SHALL use this same function, so all interfaces show the same data.

#### Scenario: Any interface refreshes

- **WHEN** any interface refreshes its view
- **THEN** it calls the single refresh path and renders the returned items

### Requirement: Ordering

The refresh SHALL order items in three sections: open items first (in task
priority order), then active items (most recently updated first), then closed
items (most recently updated first). Spec-only items, having no task, sort with
the open section.

#### Scenario: Mixed states

- **WHEN** the work list contains open, in-progress, and closed items
- **THEN** they appear grouped open → active → closed as specified

## Structure

- `internal/issue/` (`type: logic`) — the `Issue` union, its accessors, gate/
  spec/status logic, and the pure `Assemble`.
- `internal/board/` (`type: assembly`) — `List`, the single refresh path, which
  fetches from the adapters and calls `Assemble`.
