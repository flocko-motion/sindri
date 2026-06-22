# Workflow — delta

## MODIFIED Requirements

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
