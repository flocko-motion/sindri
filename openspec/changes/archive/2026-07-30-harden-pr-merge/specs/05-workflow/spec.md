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
