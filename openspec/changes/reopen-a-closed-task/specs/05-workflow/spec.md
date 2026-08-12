# Workflow — delta

## MODIFIED Requirements

### Requirement: The task lifecycle

A task SHALL travel: open → claimed (in_progress, set by `sindri-worker issue next` via
`td start`) → submitted (in_review with a PR, set by `sindri-worker submit` via `td
review`) → merged (task closed, by action-merge) or rejected (task back to open,
by action-reject). A claimed task left over from a crashed run SHALL be reset on
the next `sindri-worker issue next`. Before merging, the PR branch SHALL be rebased
onto the current base; a rebase CONFLICT SHALL return the work to its owning worker
(as a rejection does, with the conflict reported) rather than landing or failing
silently. A merge that cannot be applied because the base checkout has uncommitted
local changes is NOT the worker's to fix — it SHALL NOT reject the PR (which stays
approved) but SHALL report a clear, actionable message naming the files to commit
or stash before retrying.

A closed task sindri owns is not necessarily done travelling: it MAY be reopened
back to open (see "Reopening a closed task" below). Reopening creates no new
approval gate — it restores the release the task already had rather than granting
new work — and leaves the task's priority exactly as it stood, which alone MAY
make it immediately claimable again the moment it reopens.

#### Scenario: Happy path

- **WHEN** a worker implements an open task and submits it, and a human approves
  and merges
- **THEN** the PR lands on the base branch and the task is closed

#### Scenario: Rework path

- **WHEN** a submitted task is rejected with feedback
- **THEN** it returns to open and `sindri-worker issue next` surfaces it again with the
  rejection comment

#### Scenario: Reopen path

- **WHEN** a closed task sindri owns is reopened (see "Reopening a closed task")
- **THEN** it returns to open with no new approval gate, and a priority left
  standing from before the close carries over unchanged

#### Scenario: Orphan recovery

- **WHEN** `sindri-worker issue next` runs with a stale in_progress task from a prior run
- **THEN** that task is unstarted before a new one is claimed

#### Scenario: Rebase conflict returns to the worker

- **WHEN** the pre-merge rebase of an approved PR's branch onto the current base
  conflicts
- **THEN** the work is routed back to the owning worker to resolve and resubmit,
  just as a rejection would, with the conflict reported

#### Scenario: Dirty base checkout is reported to the human

- **WHEN** merging an approved PR fails because the base checkout has uncommitted
  local changes the merge would overwrite
- **THEN** the PR is NOT rejected (it stays approved) and the human is shown a
  clear message naming the files to commit or stash before retrying the merge

## ADDED Requirements

### Requirement: Reopening a closed task

A closed task sindri owns (an `sd-` or `td-` id) SHALL be reopenable back to
open. A `gh-` or `os-` id SHALL be refused: its status comes from its own
source (a GitHub issue, an archived openspec change), not sindri's own store,
so writing "open" here would only desync from what that source actually
reports. A task that is not currently closed SHALL be refused rather than
treated as a no-op, so a typo'd id is never mistaken for a successful reopen.

Reopening SHALL require a reason, and SHALL record it as a comment on the
task's thread — the "this did not hold" signal that tells the next reader this
was a failed verification rather than a change of mind, which a fresh
duplicate task would otherwise lose. Reopening SHALL NOT create a new approval
gate: it restores the release the task already had, not new work. The task's
priority SHALL be left exactly as it stood, which alone MAY make the task
immediately claimable by a worker the instant it reopens; the reopener SHALL be
told when that is the case, rather than discovering it.

The capability SHALL be reachable by the user, from either front-end, and by a
planner through a dedicated verb. It SHALL NOT be offered to a worker: a worker
reopening a task would let an agent undo a human's verdict on its own task.

#### Scenario: A closed task is reopened with a reason

- **WHEN** the user or a planner reopens a closed `sd-`/`td-` task, giving a
  reason
- **THEN** the task returns to open, and the reason is recorded as a comment on
  the task's thread

#### Scenario: An empty reason is refused

- **WHEN** a reopen is attempted with no reason given
- **THEN** it is refused, and nothing about the task changes

#### Scenario: A gh-/os- id is refused

- **WHEN** a reopen is attempted on a task backed by a GitHub issue or an
  openspec change
- **THEN** it is refused, naming that the task's status comes from its own
  source rather than sindri's store

#### Scenario: A task that is not closed is refused

- **WHEN** a reopen is attempted on a task that is open, in progress, or in
  review
- **THEN** it is refused rather than treated as a no-op

#### Scenario: A standing priority makes the task immediately claimable

- **WHEN** a closed task that still carries a priority is reopened
- **THEN** it becomes claimable by a worker immediately, and the reopener is
  told so

#### Scenario: A worker cannot reopen a task

- **WHEN** a worker's available commands are listed
- **THEN** reopening a task is not among them
