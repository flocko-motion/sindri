# Workflow — delta

## MODIFIED Requirements

### Requirement: Plan / build / review separation

Humans SHALL plan (author tasks and specs) and merge; the worker agent SHALL
build (implement tasks, open PRs); the reviewer agent SHALL review (approve or
reject the worker's PRs). A human MAY also approve or reject a worker's PR
directly from the host — review approval is not the reviewer agent's exclusive
power. No agent SHALL approve or merge its OWN work — review of a worker's PR is
performed by the separate reviewer agent or by a human on the host — and merge is
human-only.

#### Scenario: Roles

- **WHEN** work moves through the loop
- **THEN** tasks/specs are authored by humans, implemented by the worker agent,
  reviewed (approved/rejected) by the reviewer agent or by a human on the host,
  and merged by a human

#### Scenario: Human approves a worker's PR

- **WHEN** a human approves a worker's PR from the host
- **THEN** it is marked approved and may be merged, without requiring a reviewer
  agent to have approved it first
