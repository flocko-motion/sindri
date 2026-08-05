# gh-local — delta

## RENAMED Requirements

- FROM: `### Requirement: td reads are direct, writes go through the tool`
- TO: `### Requirement: td is a one-way import, not a backend`

## MODIFIED Requirements

### Requirement: td is a one-way import, not a backend

Sindri SHALL own its tasks in the hub's own store, and SHALL NOT depend on the td tool at
runtime. Where a repository carries an existing td database, the hub SHALL import that
backlog **once**, the first time it syncs the project, so adopting sindri costs nobody their
tasks. That import SHALL read td's own SQLite directly, and SHALL be the only way td is
touched: the td CLI SHALL NOT be invoked, and td's database SHALL NOT be written.

Because the import happens once, a task that arrives this way SHALL thereafter be an
ordinary task sindri owns, indistinguishable from one created in sindri — including its
`td-` prefix, which records sindri's ownership rather than a live backend (see `hub`).

#### Scenario: Existing backlog imported once

- **WHEN** the hub syncs a project whose repository has a td database for the first time
- **THEN** that backlog is imported into the hub's own store, and subsequent syncs import
  nothing further

#### Scenario: td is never written

- **WHEN** a task imported from td is created, changed, or closed
- **THEN** the change lands in the hub's own store, and neither td's database nor the td CLI
  is touched

#### Scenario: No td, no problem

- **WHEN** a repository has never used td
- **THEN** nothing is imported and the hub's own tasks are the whole backlog
