# Workflow

## Purpose

Ties the board (02), gh-local (03), and workers (04) into the end-to-end loop:
how a unit of work travels from a planned task to merged code. Humans plan and
review; agents build. The individual human actions are the `action-*` specs; the
agent's verbs are `sindri-worker issue next`, `sindri-worker submit`, `sindri-worker done`, `sindri-worker issue comment`.
This chapter is the integration.

## Requirements

### Requirement: Plan / build / review separation

Humans SHALL plan (create tasks and specs) and review (approve/merge); agents
SHALL build (implement tasks and open PRs). Agents SHALL NOT approve or merge
their own work.

#### Scenario: Roles

- **WHEN** work moves through the loop
- **THEN** tasks/specs are authored by humans, implemented by agents, and
  approved/merged by humans

### Requirement: The worker loop

A worker SHALL run a loop of: `sindri-worker issue next` (claim the highest-priority open
task and branch for it) → implement, test, commit → `sindri-worker submit` (lint, open a PR
and submit for review) → `sindri-worker done` (return to base) → repeat. After submitting, the
worker SHALL move on to the next task rather than block waiting for review. Submit enforces
the lint gate (see 03-gh-local), so a failing worker fixes the violations and submits again.

#### Scenario: One iteration

- **WHEN** a worker finishes a task and submits it
- **THEN** it runs `sindri-worker done` and `sindri-worker issue next` to pick up the next task
  without waiting for the review verdict

#### Scenario: Queue empty

- **WHEN** `sindri-worker issue next` finds no open task
- **THEN** the worker reports none available and waits for instructions

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

A worker that is blocked or uncertain SHALL ask by commenting on the task (`gh
issue comment -b "..."`); the human replies with another comment, and the
comment thread SHALL be shown when the task is next viewed or picked up.

#### Scenario: Blocking question

- **WHEN** a worker is unsure how to proceed
- **THEN** it comments the question on the task and the human's reply is visible
  on the next view

### Requirement: Quality gates before merge

Merge SHALL be gated on the task's review gates (see action-review-gate,
action-merge); work cannot land until every `require-review-*` gate has its
matching `approved-review-*`.

#### Scenario: Gate enforced

- **WHEN** a merge is attempted before a required review is approved
- **THEN** the merge is refused and the missing gate is named

## Structure

The agent's loop lives in `container/skills/td-next` (build) and
`container/skills/td-review` (review); the agent's verbs are implemented in
`cmd/gh/issue.go` (`next`/`list`/`view`/`comment`), `cmd/gh/submit.go`, and
`cmd/gh/done.go`, with PR records in `internal/ghlocal/store`. The human review
flow is the `action-*` specs driven from `cmd/sindri` and the TUI. Task state
transitions go through the td CLI (in-container) and the td adapter (on host).
