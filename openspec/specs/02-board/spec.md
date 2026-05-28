# Board

## Purpose

Defines the board — the unified, ordered collection of Issues that is the
application's state. Every interface renders the board; every action mutates the
things it is built from. The board and its Issues are UI-agnostic: one shape,
one refresh path, shared by all interfaces.

## Requirements

### Requirement: The Issue union

An Issue SHALL be the union of an optional task and an optional spec, in one of
three shapes: a task with no spec, a task implementing a spec, or a spec with no
task yet. At least one of the two SHALL be present.

#### Scenario: Spec with no task

- **WHEN** an openspec change has no task linked to it
- **THEN** it appears on the board as a spec-only Issue (needing a task)

#### Scenario: Task linked to a spec

- **WHEN** a task carries a `spec:<name>` label
- **THEN** its Issue also carries that spec

### Requirement: Stable identity

Every Issue SHALL expose a stable id: the task id (`td-……`) for an Issue with a
task, or a derived `os-……` id for a spec-only Issue.

#### Scenario: Spec-only id

- **WHEN** a spec-only Issue is shown
- **THEN** its id is a stable `os-……` derived from the spec name

### Requirement: Review gates

An Issue SHALL derive its review gates from the task's labels: each
`require-review-<x>` is a gate, satisfied when a matching `approved-review-<x>`
is present. The set of unmet gates SHALL be queryable.

#### Scenario: Unmet gate

- **WHEN** a task has `require-review-code` but not `approved-review-code`
- **THEN** the Issue reports one unmet gate

### Requirement: Single refresh path

The board SHALL be produced by one refresh function that gathers tasks, specs,
worker assignments, and PRs and assembles them into Issues. Every interface
SHALL obtain the board from this same function, so all interfaces show the same
data; refreshing re-runs it rather than mutating in place.

#### Scenario: Any interface refreshes

- **WHEN** any interface refreshes
- **THEN** it rebuilds the board from the single refresh path and renders it

### Requirement: Ordering

The board SHALL order Issues in three sections: open Issues first (in task
priority order), then active Issues (most recently updated first), then closed
Issues (most recently updated first). Spec-only Issues, having no task, sort
with the open section.

#### Scenario: Mixed states

- **WHEN** the board contains open, in-progress, and closed Issues
- **THEN** they appear grouped open → active → closed as specified

## Structure

- `internal/issue/` (`type: logic`) — the `Issue` union, its accessors, gate/
  spec/status logic, and the pure `Assemble` that builds the board.
- `internal/board/` (`type: assembly`) — `List`, the single refresh path, which
  fetches from the adapters and calls `Assemble`.
