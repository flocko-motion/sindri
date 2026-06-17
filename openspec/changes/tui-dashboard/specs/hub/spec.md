# Hub — delta

## ADDED Requirements

### Requirement: Sections with actionable counts

The hub SHALL expose a section model — an ordered set of sections, each with a
key, a title, and a count derived from board state — as the single source of
truth for which views exist and the badge each shows. The counts SHALL be the
actionable subset: non-closed tasks, running agents, and not-merged PRs. UIs
SHALL render these counts rather than computing their own.

#### Scenario: A UI renders section counts

- **WHEN** a UI draws its tabs
- **THEN** each tab's badge is the hub-provided count for that section

#### Scenario: Adding a section

- **WHEN** a new section is introduced
- **THEN** it is added to the hub's section model and UIs pick it up without
  re-deriving counts

### Requirement: Task hierarchy arrangement

The hub SHALL arrange a flat set of tasks into their parent/child tree — roots
ordered by priority, each followed by its descendants, with a depth per node —
and annotate each with the id of a non-merged PR for that task, if any. A task
whose parent is absent from the set SHALL be arranged as a root. This arrangement
SHALL be a logic-layer function so every UI renders the same tree.

#### Scenario: Tree with depth

- **WHEN** tasks with parent relationships are arranged
- **THEN** the result lists each parent before its children with an increasing
  depth, and standalone tasks at depth zero

#### Scenario: PR annotation

- **WHEN** a task has a non-merged PR
- **THEN** its arranged row carries that PR's id

### Requirement: Board carries all tasks with hierarchy

The board state the hub serves SHALL include all tasks (every status), each with
its parent and a description, so a UI can show what is being worked — by whom, in
its hierarchy — and can filter to open/closed/all client-side. Section counts
SHALL derive the non-closed subset from this full set.

#### Scenario: In-progress and closed tasks both present

- **WHEN** the board is requested
- **THEN** it includes in_progress tasks (with parent + assignable detail) and
  closed tasks, so a UI can filter between them without another fetch

### Requirement: PR detail includes its linked task

A PR's detail from the hub SHALL include the linked task (id, title, status) in
addition to the diff, resolved from the source of truth so it is present even
after the task closes on merge.

#### Scenario: PR detail carries the task

- **WHEN** a PR's detail is requested
- **THEN** it includes the linked task's id, title, and status, and the diff
