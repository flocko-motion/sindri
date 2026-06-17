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
the next `sindri-worker issue next`.

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

## Structure

The agent loops live in `container/skills/td-next` (worker) and
`container/skills/td-review` (reviewer); the agents' verbs are implemented in
`internal/agentcli` (issue/submit/done/pr…), wired into the `sindri-worker` and
`sindri-review` binaries, with PR records in `internal/ghlocal/store`. The
human-only merge and the host review flow are the `action-*` specs driven from
`cmd/sindri` and the TUI. Task state transitions go through the td CLI
(in-container) and the td adapter (on host).
