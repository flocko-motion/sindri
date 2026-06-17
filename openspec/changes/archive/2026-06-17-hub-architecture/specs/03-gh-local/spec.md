# gh-local — delta

## ADDED Requirements

### Requirement: td reads are direct, writes go through the tool

For the td backend, the td adapter SHALL read tasks directly from td's own SQLite
database for speed, but SHALL perform every write action (create, start, comment,
review, …) only through the `td` tool — never by writing td's database directly.
Both strategies SHALL be encapsulated in `internal/adapter/td` so internal logic
sees a single adapter interface.

#### Scenario: Fast read

- **WHEN** the hub syncs td tasks into its cache
- **THEN** it reads td's SQLite directly rather than invoking the td CLI per query

#### Scenario: Write through the tool

- **WHEN** a td task is created or mutated
- **THEN** the change goes through the `td` tool, never a direct write to td's DB

## MODIFIED Requirements

### Requirement: PRs are local records

A pull request SHALL be a merge-intent owned by the hub: a branch, a flag meaning
"the agent would like this branch merged," and a verdict. The hub SHALL hold this
state; agents SHALL NOT write a PR store. There SHALL NOT be a separate `.git/pr`
record store.

#### Scenario: Registering merge-intent

- **WHEN** an agent registers its branch for merge
- **THEN** the hub records the intent (branch + wants-merge + pending verdict) in
  its own state, and the call returns immediately

### Requirement: Role-scoped commands; merge is human-only

The agent client SHALL be a single role-agnostic browser whose available commands
are filtered by the hub from the caller's role and state. A worker's surface SHALL
expose registering and inspecting merge-intents but never approve/reject/merge; a
reviewer's surface SHALL expose approve/reject but never submit. Merge SHALL be
human-only, exposed only on the host and requiring explicit confirmation; no agent
surface SHALL ever include merge.

#### Scenario: Reviewer approves, human merges

- **WHEN** the reviewer approves a PR
- **THEN** the hub marks it approved and its gates satisfied, but it is merged only
  later by a human on the host

#### Scenario: No agent merge

- **WHEN** any agent queries its command surface
- **THEN** no merge command appears; only the host `sindri pr merge` can merge,
  after human confirmation
