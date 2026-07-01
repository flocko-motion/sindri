# gh-local — delta

## MODIFIED Requirements

### Requirement: Role-scoped commands; merge is human-only

The agent client SHALL be a single role-agnostic browser whose available commands
are filtered by the hub from the caller's role and state. A worker's surface SHALL
expose registering and inspecting merge-intents but never approve/reject/merge; a
reviewer's surface SHALL expose approve/reject but never submit; a coauthor's
surface SHALL expose only the generic helpers (status, log, lint, and the
read-only PR views) and none of the build or review verbs — a coauthor commits
with git directly rather than through a hub verb. Merge SHALL be human-only,
exposed only on the host and requiring explicit confirmation; no agent surface
SHALL ever include merge.

#### Scenario: Reviewer approves, human merges

- **WHEN** the reviewer approves a PR
- **THEN** the hub marks it approved and its gates satisfied, but it is merged only
  later by a human on the host

#### Scenario: Coauthor has only helpers

- **WHEN** a coauthor queries its surface
- **THEN** it sees only status/log/lint and the read-only PR views, with no
  `next`/`submit`, no `approve`/`reject`, no planner verbs, and no merge

#### Scenario: No agent merge

- **WHEN** any agent queries its command surface
- **THEN** no merge command appears; only the host `sindri pr merge` can merge,
  after human confirmation
