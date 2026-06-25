# Hub — delta

## MODIFIED Requirements

### Requirement: Command surface is state-filtered

The hub SHALL compute the set of commands available to a caller from its role and
current state, and the commands endpoint SHALL return only what is possible right
now. A command that is not currently valid SHALL NOT appear, so an out-of-order
action is invisible rather than rejected. The hub SHALL recognise three roles —
worker, reviewer, and planner — and the surface SHALL be scoped to each: a worker
registers and inspects merge-intents (`next`/`submit`); a reviewer judges them
(`approve`/`reject`/`review`); a planner reads the backlog and proposes work
(`task`/`create-task`/`openspec`). No role SHALL ever see merge.

#### Scenario: Blocked-on-PR worker

- **WHEN** a worker has a branch awaiting a merge verdict
- **THEN** "pick up the next task" is absent from its command surface until the
  verdict arrives

#### Scenario: Reviewer never sees submit

- **WHEN** a reviewer queries its command surface
- **THEN** worker-only verbs such as submit are absent from it

#### Scenario: Planner surface is propose-and-ship

- **WHEN** a planner queries its command surface
- **THEN** it sees `task`, `create-task`, and `openspec` but never the worker's
  `next`/`submit` nor the reviewer's `approve`/`reject`

## ADDED Requirements

### Requirement: Planner task proposals are gated on user approval

A planner SHALL propose backlog tasks with `create-task`, but a proposed task
SHALL NOT be claimable by any worker until the user approves it. The hub SHALL
record a per-task approval state — pending, approved, or rejected — held in
`hub.db` separate from the task's own status. A task with no approval row is a
normal, claimable task; a task flagged pending or rejected SHALL be hidden from
the work an agent can claim. Approval and rejection SHALL be user-only actions
(`sindri task approve`/`reject`), and the hub SHALL inject the verdict into every
running planner's session.

#### Scenario: Proposed task is withheld until approved

- **WHEN** a planner runs `create-task`
- **THEN** the task is created in the backend flagged pending the user's approval,
  and no worker can claim it while it is pending

#### Scenario: User approves a proposal

- **WHEN** the user approves a planner-proposed task
- **THEN** the approval gate clears, the task becomes claimable by a worker, and
  any running planner is told it was approved

#### Scenario: User rejects a proposal

- **WHEN** the user rejects a planner-proposed task with a comment
- **THEN** the task stays hidden from workers and the comment is injected into any
  running planner's session
