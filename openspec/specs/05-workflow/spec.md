# Workflow

## Purpose

Ties the board (02), gh-local (03), and workers (04) into the end-to-end loop:
how a unit of work travels from a planned task to merged code. Humans plan and
review; agents build. The individual human actions are the `action-*` specs; the
agent's verbs are `sindri-worker issue next`, `sindri-worker submit`, `sindri-worker done`, `sindri-worker issue comment`.
This chapter is the integration.
## Requirements
### Requirement: Plan / build / review separation

Work SHALL be separated into planning, building, and reviewing. The planner agent
SHALL shape upcoming work *with* the user — reading the repo and specs, proposing
backlog tasks, and drafting openspec — but the user SHALL retain the gates: only
the user approves a proposed task into the backlog, and only the user merges. The
worker agent SHALL build (implement tasks, open PRs); the reviewer agent SHALL
review (approve or reject the worker's PRs). A human MAY also approve or reject a
worker's PR directly from the host — review approval is not the reviewer agent's
exclusive power. No agent SHALL approve or merge its OWN work — review of a
worker's PR is performed by the separate reviewer agent or by a human on the host
— and merge is human-only.

#### Scenario: Roles

- **WHEN** work moves through the loop
- **THEN** the planner drafts specs and proposes tasks with the user, the user
  approves tasks and merges, the worker implements approved tasks and opens PRs,
  and the reviewer (or a human on the host) approves or rejects those PRs

#### Scenario: Human approves a worker's PR

- **WHEN** a human approves a worker's PR from the host
- **THEN** it is marked approved and may be merged, without requiring a reviewer
  agent to have approved it first

#### Scenario: Planner cannot self-serve work

- **WHEN** a planner proposes a task
- **THEN** the task is not claimable until the user approves it, so the planner
  cannot inject work into the backlog unilaterally

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

### Requirement: The coauthor works outside the managed loop

A coauthor SHALL work directly with the user, outside the managed
plan/build/review loop. It SHALL NOT claim backlog tasks, SHALL NOT open managed
PRs, and SHALL NOT pass through a review gate. It shares the user's checkout, so
the user steers and reviews its work directly in the same tree, and the coauthor
SHALL use git itself — the hub does not commit on its behalf as it does for a
worker, and there is no merge-intent to approve. This is the freestyle
counterpart to the gated worker loop; the two coexist, and the user chooses per
agent which mode they want.

#### Scenario: Coauthor takes no managed task

- **WHEN** a coauthor is working with the user
- **THEN** it never claims a backlog task or registers a merge-intent; the work is
  driven entirely by the user in the shared checkout

#### Scenario: Coauthor work is not gated by review

- **WHEN** a coauthor changes code in the shared checkout
- **THEN** the change is not routed through a reviewer or a merge gate; the user
  reviews it directly, since they share the tree

#### Scenario: Coauthor commits with git itself

- **WHEN** a coauthor needs to commit
- **THEN** it runs git directly in `/workspace`, rather than asking the hub to
  commit and submit a branch the way a worker does

### Requirement: The planner loop

A planner SHALL run a loop of: orient (read README, the backlog, the specs) → wait
for the user to steer it → with the user, propose tasks (`create-task`) and draft
openspec → ship the specs for review (`openspec submit`) → go idle. A planner
SHALL NOT claim backlog tasks and SHALL NOT block; like a worker, it goes idle
after shipping and is woken by the hub injecting a verdict or user steering. A
proposed task SHALL require the user's approval before any worker can claim it.

#### Scenario: Orient then wait

- **WHEN** a planner is launched or has nothing in flight
- **THEN** it is directed to read the repo and specs and then wait for the user,
  rather than being assigned a backlog task

#### Scenario: Propose and ship

- **WHEN** a planner, working with the user, has drafted specs and proposed tasks
- **THEN** the tasks await the user's approval and the specs are shipped via
  `openspec submit` for review, after which the planner goes idle

#### Scenario: Rejected plan

- **WHEN** a planner's shipped openspec PR is rejected by the reviewer
- **THEN** the planner drops to idle and the feedback is injected so it can revise
  and submit again

### Requirement: The orchestrator sequences a session reset itself

The orchestrator SHALL issue a harness command, wait for its answer, and then deliver the following
instruction itself through the ordinary delivery path, wherever it needs an agent's session reset —
cleared, compacted, or moved to another model — before that instruction lands. It SHALL NOT hand the
instruction to the harness command to be sent on its behalf.

A harness command that fails or times out SHALL leave the orchestrator holding the decision about
what happens next, and the agent SHALL NOT be left both un-reset and un-instructed.

Because the reset completes within the call that asked for it, there SHALL be no directive whose
purpose is to answer an agent that asks for work while a reset is pending, and no stored window
marking an assignment as in-flight so that the boundary check will admit it.

#### Scenario: A cleared agent is instructed by the orchestrator

- **WHEN** the orchestrator clears an agent's session as part of handing it work
- **THEN** it waits for the clear to answer, and then delivers the agent's directive itself

#### Scenario: A failed reset is the orchestrator's to answer for

- **WHEN** the orchestrator clears an agent's session and the command answers with a failure
- **THEN** the orchestrator decides what follows — retrying, escalating, or leaving the agent as it
  was — rather than the failure being recorded where no caller reads it

#### Scenario: Nothing asks about a pending reset

- **WHEN** an agent asks the hub for its next action
- **THEN** it is never answered with a notice that a reset is about to land, because a reset in
  progress is inside a call that has not yet returned

## Structure

The loops are the hub's, not the agents': `internal/hub/workflow/` decides what each
role is told to do next, and `cmd/sindri-worker/` is the thin browser that renders it
— one binary for every role, its surface filtered by `internal/hub/commands/`. PR
records and task state live in `internal/hub/store/`, and the trackers are reached
through `internal/adapter/tasks/` (`td`, `spec`, `github`). The human-only merge and
the host review flow are driven from `cmd/sindri` and the TUI in `internal/ui/`.
