# gh-local — delta

## MODIFIED Requirements

### Requirement: Role-scoped commands; merge is human-only

The agent client SHALL be a single role-agnostic browser whose available commands
are filtered by the hub from the caller's role and state. A worker's surface SHALL
expose registering and inspecting merge-intents but never approve/reject/merge; a
reviewer's surface SHALL expose approve/reject but never submit; a planner's
surface SHALL expose reading the backlog, proposing tasks, and shipping openspec
(`task`/`create-task`/`openspec`) but never the worker's `next`/`submit` nor the
reviewer's `approve`/`reject`. Merge SHALL be human-only, exposed only on the host
and requiring explicit confirmation; no agent surface SHALL ever include merge.

#### Scenario: Reviewer approves, human merges

- **WHEN** the reviewer approves a PR
- **THEN** the hub marks it approved and its gates satisfied, but it is merged only
  later by a human on the host

#### Scenario: Planner ships, not builds

- **WHEN** a planner queries its surface
- **THEN** it can read the backlog, propose tasks, and ship openspec, but it has no
  `next`/`submit`/`approve`/`reject`, and no merge

#### Scenario: No agent merge

- **WHEN** any agent queries its command surface
- **THEN** no merge command appears; only the host `sindri pr merge` can merge,
  after human confirmation

## ADDED Requirements

### Requirement: Planner ships openspec changes as a PR

A planner SHALL turn its openspec edits into a merge-intent with `openspec submit`,
reviewed and merged through the same cycle as a worker's PR. The planner SHALL work
on a standing branch (`plan-<name>`) rather than a per-task branch, and its PR SHALL
carry no real backlog task (a placeholder task id stands in for it). Submitting
SHALL run the same lint gate as a worker's submit — including openspec validation —
and refuse the PR if a gate fails. On reviewer rejection the planner SHALL drop to
idle with the feedback injected; after any merge moves the base branch, every
planner's standing branch SHALL be rebased onto the new base so planners stay
current.

#### Scenario: Shipping a plan

- **WHEN** a planner runs `openspec submit` with openspec edits that pass the gate
- **THEN** its standing branch is committed and a merge-intent is registered,
  reviewed like a worker's PR, with no backlog task behind it

#### Scenario: Plan fails the gate

- **WHEN** a planner submits openspec that fails the lint gate (e.g. invalid spec)
- **THEN** no PR is created and the violations are reported for the planner to fix

#### Scenario: Planner rebased after a merge

- **WHEN** a PR merges and moves the base branch
- **THEN** each planner's standing branch is rebased onto the new base so it sees
  the latest code
