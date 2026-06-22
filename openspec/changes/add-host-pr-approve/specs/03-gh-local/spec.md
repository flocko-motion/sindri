# gh-local — delta

## MODIFIED Requirements

### Requirement: Role-scoped commands; merge is human-only

The agent client SHALL be a single role-agnostic browser whose available commands
are filtered by the hub from the caller's role and state. A worker's surface SHALL
expose registering and inspecting merge-intents but never approve/reject/merge; a
reviewer's surface SHALL expose approve/reject but never submit; a planner's
surface SHALL expose reading the backlog, proposing tasks, and shipping openspec
(`task`/`create-task`/`openspec`) but never the worker's `next`/`submit` nor the
reviewer's `approve`/`reject`. Approval SHALL NOT be the reviewer agent's
exclusive power: the host SHALL also expose a human approve (`sindri pr approve`),
the positive counterpart of the existing human reject, so a PR can reach
`approved` without a reviewer agent in the loop. A human approve SHALL mark the PR
approved and satisfy its review gates exactly as a reviewer approve does, and SHALL
apply only to an open PR (one awaiting a verdict). Merge SHALL be human-only,
exposed only on the host and requiring explicit confirmation; no agent surface
SHALL ever include merge.

#### Scenario: Reviewer approves, human merges

- **WHEN** the reviewer approves a PR
- **THEN** the hub marks it approved and its gates satisfied, but it is merged only
  later by a human on the host

#### Scenario: Human approves without a reviewer

- **WHEN** no reviewer agent has approved a PR and the user approves it on the host
- **THEN** the hub marks it approved and its gates satisfied, so the user can then
  merge it — a reviewer agent is not required to reach `approved`

#### Scenario: Approve only an open PR

- **WHEN** a human approve targets a PR that is not open (already approved, merged,
  or rejected)
- **THEN** the approve is refused and the PR's current status is reported, mirroring
  the reviewer approve's open-only guard

#### Scenario: Planner ships, not builds

- **WHEN** a planner queries its surface
- **THEN** it can read the backlog, propose tasks, and ship openspec, but it has no
  `next`/`submit`/`approve`/`reject`, and no merge

#### Scenario: No agent merge

- **WHEN** any agent queries its command surface
- **THEN** no merge command appears; only the host `sindri pr merge` can merge,
  after human confirmation
