# Workflow — delta

## ADDED Requirements

### Requirement: A worker past its context threshold is not auto-assigned

The automatic assigner SHALL NOT hand a leaf task or a container to a worker
whose current context size is at or past a fixed threshold. This applies
before either the leaf or the container assignment path runs, so a full worker
is skipped for both alike. A worker with no context measurement yet SHALL NOT
be treated as full. A worker skipped for this reason SHALL be told directly,
rather than left waiting on the same "no work" response an empty queue gives.

#### Scenario: A full worker is skipped by the assigner

- **WHEN** an idle worker whose context is past the threshold asks for its next
  task, and an open leaf or an unheld container exists
- **THEN** the assigner does not claim either for it, and it is told its
  context is full rather than that there is no work

#### Scenario: An unmeasured worker is assigned normally

- **WHEN** an idle worker with no context measurement yet asks for its next
  task
- **THEN** the assigner claims for it exactly as it would for any worker under
  the threshold
