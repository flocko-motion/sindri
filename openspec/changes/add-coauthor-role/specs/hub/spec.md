# Hub — delta

## MODIFIED Requirements

### Requirement: Command surface is state-filtered

The hub SHALL compute the set of commands available to a caller from its role and
current state, and the commands endpoint SHALL return only what is possible right
now. A command that is not currently valid SHALL NOT appear, so an out-of-order
action is invisible rather than rejected. The hub SHALL recognise four roles —
worker, reviewer, planner, and coauthor — and scope the surface to each. A
coauthor, which works freestyle with the user, SHALL see only the generic helper
verbs (status, log, lint, and the read-only PR views) and none of the workflow
verbs — not the worker's `next`/`submit`/`checkpoint`, the reviewer's
`approve`/`reject`/`review`, nor the planner's `task`/`create-task`/`openspec`.

#### Scenario: Blocked-on-PR worker

- **WHEN** a worker has a branch awaiting a merge verdict
- **THEN** "pick up the next task" is absent from its command surface until the
  verdict arrives

#### Scenario: Reviewer never sees submit

- **WHEN** a reviewer queries its command surface
- **THEN** worker-only verbs such as submit are absent from it

#### Scenario: Coauthor surface is helpers only

- **WHEN** a coauthor queries its command surface
- **THEN** it sees only the generic helpers (status, log, lint, read-only PR views)
  and none of the worker, reviewer, or planner workflow verbs
