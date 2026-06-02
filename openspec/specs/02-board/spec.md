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

The board SHALL place spec-only Issues first (they have no task yet), then task
Issues grouped open → active → closed: open Issues in the order the task source
returns them (td's default ordering), then active Issues (most recently updated
first), then closed Issues (most recently updated first).

#### Scenario: Mixed states

- **WHEN** the board contains a spec-only Issue plus open, in-progress, and
  closed tasks
- **THEN** the spec-only Issue appears first, then the tasks grouped
  open → active → closed

### Requirement: Issues expose task hierarchy

Each `Issue` whose backing task has a `parent_id` SHALL expose that parent id,
and the board's refresh SHALL enrich tasks with the `parent_id` field td
reveals only on `td show --json` (since `td list --json` strips it). The board
SHALL also stamp each Issue with a `Depth` derived from its position in the
hierarchy (root = 0, child = 1, …) so renderers can indent uniformly.

#### Scenario: Refresh enriches parent_id

- **WHEN** the board refreshes
- **THEN** each Issue whose task has a parent has that `parent_id` populated,
  not left empty

#### Scenario: Children sit immediately after their parent

- **WHEN** the board is assembled with parent/child relationships
- **THEN** each child Issue appears immediately after its parent in the
  resulting `[]Issue`, in depth-first order, with `Depth` set to one more
  than its parent's

## Structure

- `internal/issue/` (`type: logic`) — the `Issue` union, its accessors, gate/
  spec/status logic, and the pure `Assemble` that builds the board.
- `internal/board/` (`type: assembly`) — `List`, the single refresh path, which
  fetches from the adapters and calls `Assemble`.
