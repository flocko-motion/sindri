# Workflow

## Purpose

Ties the board (02), gh-local (03), and workers (04) into the end-to-end loop:
how a unit of work travels from a planned task to merged code. Humans plan and
review; agents build. The individual human actions are the `action-*` specs; the
agent's verbs are `sindri-worker issue next`, `sindri-worker submit`, `sindri-worker done`, `sindri-worker issue comment`.
This chapter is the integration.
## Requirements
### Requirement: Plan / build / review separation

Humans SHALL plan (author tasks and specs) and merge; the worker agent SHALL
build (implement tasks, open PRs); the reviewer agent SHALL review (approve or
reject the worker's PRs). No agent SHALL approve or merge its OWN work — review
is performed by the separate reviewer agent, and merge is human-only.

#### Scenario: Roles

- **WHEN** work moves through the loop
- **THEN** tasks/specs are authored by humans, implemented by the worker agent,
  reviewed (approved/rejected) by the reviewer agent, and merged by a human

### Requirement: The worker loop

A worker SHALL run a loop of: receive a task (injected by the hub) → implement,
test, commit → register the branch for merge → go idle. Registering for merge
SHALL return immediately; the worker SHALL NOT block waiting for the verdict and
SHALL NOT poll. The hub SHALL wake the worker by injecting the next task or the
verdict when ready. Idle is the worker's resting state, and a long wait is
expected.

#### Scenario: One iteration

- **WHEN** a worker finishes a task and registers it for merge
- **THEN** the call returns at once and the worker goes idle until the hub injects
  the next task or a verdict

#### Scenario: Queue empty

- **WHEN** there is no open task for a worker
- **THEN** the worker simply stays idle; the hub injects a task when one appears

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

### Requirement: Spec-driven when present

When a task carries a `spec:<name>` label, the worker SHALL run `openspec show
<name>` and implement to satisfy that spec, and the reviewer SHALL verify the
diff against every requirement and scenario in the spec, rejecting work that
compiles but does not meet the spec.

#### Scenario: Linked task built

- **WHEN** a worker picks up a task labeled `spec:add-auth`
- **THEN** it reads that spec first and implements to satisfy it

#### Scenario: Linked task reviewed

- **WHEN** a reviewer examines a PR for a spec-linked task
- **THEN** it checks the diff against the spec and rejects if any requirement is
  unmet

### Requirement: Communication via comments

Messages to an agent SHALL be delivered by the hub injecting them into the
agent's session, each stamped with its source. A human reaches an agent via the
hub-mediated `tell` channel; another agent reaches it only by acting on a shared
object whose consequence the hub routes. The agent SHALL see these as tagged
lines in its single input stream.

#### Scenario: Human nudge

- **WHEN** a human sends a message to an agent
- **THEN** the hub injects it into that agent's session tagged `[user]`

#### Scenario: Reviewer feedback

- **WHEN** the reviewer rejects an agent's PR with feedback
- **THEN** the hub routes the feedback to the owning agent's session tagged
  `[reviewer]`

### Requirement: Quality gates before merge

Merge SHALL be gated on the task's review gates (see action-review-gate,
action-merge); work cannot land until every `require-review-*` gate has its
matching `approved-review-*`.

#### Scenario: Gate enforced

- **WHEN** a merge is attempted before a required review is approved
- **THEN** the merge is refused and the missing gate is named

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

## Structure

The agent loops live in `container/skills/td-next` (worker) and
`container/skills/td-review` (reviewer); the agents' verbs are implemented in
`internal/agentcli` (issue/submit/done/pr…), wired into the `sindri-worker` and
`sindri-review` binaries, with PR records in `internal/ghlocal/store`. The
human-only merge and the host review flow are the `action-*` specs driven from
`cmd/sindri` and the TUI. Task state transitions go through the td CLI
(in-container) and the td adapter (on host).
