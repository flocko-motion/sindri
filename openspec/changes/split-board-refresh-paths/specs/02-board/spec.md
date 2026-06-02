## MODIFIED Requirements

### Requirement: Single refresh path

The board SHALL be produced by **per-source loaders** that each fetch one of
the four data inputs — tasks (from td), specs (from openspec), worker
assignments (from podman), PRs (from the local store) — and that each emit
their own message. Every interface SHALL keep its `boardData` snapshot (the
four inputs) up to date as messages arrive, and SHALL re-run `issue.Assemble`
each time so the `[]issue.Issue` it renders reflects the latest data from
every source. There is still **one Assemble function** that defines how
Issues are constructed; what changes is that the loaders no longer wait for
each other before any of them can update state.

#### Scenario: Tasks land first

- **WHEN** the TUI starts and the loaders run in parallel
- **THEN** the tasks loader's message lands first (it is the fastest source)
  and the list paints immediately with what the tasks alone describe; specs,
  workers, and PRs enrich the rows as their messages arrive afterwards

#### Scenario: Post-mutation refresh

- **WHEN** the user mutates a task (move, status change, comment, ...)
- **THEN** only the tasks loader runs in the background; podman and openspec
  are not contacted because nothing about them changed

#### Scenario: Periodic refresh

- **WHEN** the periodic tick fires
- **THEN** all four loaders are dispatched; each updates `boardData` and
  triggers a reassembly as it returns
