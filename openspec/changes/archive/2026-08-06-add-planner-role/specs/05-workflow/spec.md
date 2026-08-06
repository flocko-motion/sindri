# Workflow — delta

## MODIFIED Requirements

### Requirement: Plan / build / review separation

Work SHALL be separated into planning, building, and reviewing. The planner agent
SHALL shape upcoming work *with* the user — reading the repo and specs, proposing
backlog tasks, and drafting openspec — but the user SHALL retain the gates: only
the user approves a proposed task into the backlog, and only the user merges. The
worker agent SHALL build (implement tasks, open PRs); the reviewer agent SHALL
review (approve or reject the worker's PRs). No agent SHALL approve or merge its
OWN work — review is performed by the separate reviewer agent, and merge is
human-only.

#### Scenario: Roles

- **WHEN** work moves through the loop
- **THEN** the planner drafts specs and proposes tasks with the user, the user
  approves tasks and merges, the worker implements approved tasks and opens PRs,
  and the reviewer approves or rejects those PRs

#### Scenario: Planner cannot self-serve work

- **WHEN** a planner proposes a task
- **THEN** the task is not claimable until the user approves it, so the planner
  cannot inject work into the backlog unilaterally

## ADDED Requirements

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
