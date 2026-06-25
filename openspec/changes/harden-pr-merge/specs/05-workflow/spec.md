# Workflow — delta

## MODIFIED Requirements

### Requirement: The task lifecycle

A task SHALL travel: open → claimed (in_progress, set by `sindri-worker issue next` via
`td start`) → submitted (in_review with a PR, set by `sindri-worker submit` via `td
review`) → merged (task closed, by action-merge) or rejected (task back to open,
by action-reject). A claimed task left over from a crashed run SHALL be reset on
the next `sindri-worker issue next`. Before merging, the PR branch SHALL be rebased
onto the current base. A merge that still cannot be applied for any technical
reason — a rebase conflict, or the base checkout being unable to take the merge —
SHALL return the work to its owning worker (as a rejection does, with the technical
reason and a note that the rebase was performed) rather than landing, failing
silently, or stranding an approved-but-unmergeable PR on the human.

#### Scenario: Happy path

- **WHEN** a worker implements an open task and submits it, and a human approves
  and merges
- **THEN** the PR lands on the base branch and the task is closed

#### Scenario: Rework path

- **WHEN** a submitted task is rejected with feedback
- **THEN** it returns to open and `sindri-worker issue next` surfaces it again with the
  rejection comment

#### Scenario: Orphan recovery

- **WHEN** `sindri-worker issue next` runs with a stale in_progress task from a prior run
- **THEN** that task is unstarted before a new one is claimed

#### Scenario: Unmergeable PR returns to the worker

- **WHEN** merging an approved PR cannot be applied for a technical reason — its
  branch conflicts with the current base, or the base checkout cannot take the
  merge
- **THEN** the work is routed back to the owning worker to resolve and resubmit,
  just as a rejection would, reporting the technical reason and noting that a
  rebase onto the current base was performed
