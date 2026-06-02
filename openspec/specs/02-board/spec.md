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

The board SHALL be produced by **per-source loaders** that each fetch one of
the four data inputs — tasks (from td), specs (from openspec), worker
assignments (from podman), PRs (from the local store) — and that each emit
their own message. Every interface SHALL keep its `boardData` snapshot (the
four inputs) up to date as messages arrive, and SHALL re-run `issue.Assemble`
each time so the `[]issue.Issue` it renders reflects the latest data from
every source. There is still **one Assemble function** that defines how
Issues are constructed; what changes is that the loaders no longer wait for
each other before any of them can update state.

#### Scenario: Tasks land first

- **WHEN** the TUI starts and the loaders run in parallel
- **THEN** the tasks loader's message lands first (it is the fastest source)
  and the list paints immediately with what the tasks alone describe; specs,
  workers, and PRs enrich the rows as their messages arrive afterwards

#### Scenario: Post-mutation refresh

- **WHEN** the user mutates a task (move, status change, comment, ...)
- **THEN** only the tasks loader runs in the background; podman and openspec
  are not contacted because nothing about them changed

#### Scenario: Periodic refresh

- **WHEN** the periodic tick fires
- **THEN** all four loaders are dispatched; each updates `boardData` and
  triggers a reassembly as it returns

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
