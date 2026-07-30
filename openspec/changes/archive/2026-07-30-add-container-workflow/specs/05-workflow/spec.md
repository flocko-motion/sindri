# Workflow — delta

## ADDED Requirements

### Requirement: Leaf-only auto-assignment

The automatic assigner SHALL claim only leaf tasks — those with no children. A
task that has children SHALL NOT be auto-assigned; its leaves are the unit of
automatic work. This holds for both the structured loop and the container
workflow below.

#### Scenario: Parent skipped by the assigner

- **WHEN** the next task to auto-assign would be one that has children
- **THEN** the assigner skips it and claims a leaf instead, never branching a
  parent on its own

### Requirement: Collaborative assignment of a marked container

A free agent SHALL be assignable a whole container as a unit: any task with
children MAY be marked as collaboratively assignable, and once marked its open
leaf children become that agent's reserved subtask stream — and SHALL NOT be
independently auto-assigned to other agents while the container is held. An
unmarked parent is never assigned (only its leaves are, independently).

#### Scenario: Marked container handed to one agent

- **WHEN** a marked container is picked up by a free agent
- **THEN** the agent holds the container and its open children are reserved to that
  agent, not auto-claimed elsewhere

#### Scenario: Subtask source spans bulk and interactive

- **WHEN** a container's children are pre-filled and the agent is left to run, OR a
  human feeds children to the agent live
- **THEN** the same loop applies — the difference is only who supplies the subtasks
  and when a PR is taken

### Requirement: Non-blocking checkpoints

In the container workflow, completing a subtask SHALL be a non-blocking
checkpoint: the hub commits the work to the container branch, closes that child,
and advances the agent to the next open child. The agent SHALL NOT block waiting
for a review verdict between subtasks.

#### Scenario: Checkpoint and continue

- **WHEN** an agent finishes a subtask of its container
- **THEN** the work is committed, the child is closed, and the agent immediately
  receives the next open child without waiting for a verdict

#### Scenario: Subtask stream exhausted

- **WHEN** a container has no open children left
- **THEN** the agent goes idle (awaiting more children or a milestone PR), rather
  than blocking on a verdict

### Requirement: Milestone PRs

A milestone PR SHALL capture the current state of a container branch for the
human to review and merge, and SHALL block the agent until that merge lands —
which keeps the worktree quiet so the merge and the rebase are safe. The merge
SHALL land the current state on base, rebase the branch onto the new base, and
then the agent SHALL resume the same container — it is NOT freed to take new work
and the branch is NOT retired. A milestone PR MAY be triggered on request (the
human, at a checkpoint) or automatically when the container's open children are
exhausted. Merge stays human-only and no agent merges its own work; an agent
reviewer's opinion is advisory and optional — the human's review-and-merge is the
gate.

#### Scenario: Blocking milestone, then resume

- **WHEN** a milestone PR is opened for a held container
- **THEN** the agent waits while the human reviews and merges it, and once merged
  the branch is rebased onto the new base and the agent resumes the same container

#### Scenario: PR on request (interactive)

- **WHEN** a human requests a PR for a held container at a checkpoint
- **THEN** a milestone PR is opened for the branch's current state and the agent
  waits for the human to review and merge it

#### Scenario: PR on completion (bulk)

- **WHEN** a container's last open child is closed
- **THEN** a milestone PR may be opened automatically for the human to review and
  merge

#### Scenario: Advisory reviewer

- **WHEN** an agent review is requested in the container workflow
- **THEN** the reviewer's opinion is delivered as feedback and is not a gate — the
  human's review and merge is what lands the work
