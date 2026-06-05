# Workers — delta

## ADDED Requirements

### Requirement: Role-driven launch

An agent's container SHALL be launched from a single role-parameterised path. A
`RoleSpec` SHALL describe, per role, the binary mounted, the workspace/repo mount
topology and their read/write modes, whether a base branch is provided, and the
bootstrap mode. The role SHALL be taken from the agent's index entry, not
inferred from container or worktree position.

#### Scenario: Worker role

- **WHEN** an agent with role `worker` is launched
- **THEN** its own worktree is mounted `:rw`, the repo `:ro`, `GH_LOCAL_BASE` is
  set, and the worker binary is mounted

#### Scenario: Reviewer role

- **WHEN** an agent with role `reviewer` is launched
- **THEN** the whole repo is mounted as its workspace `:ro`, no base branch env is
  set, and the review binary is mounted

### Requirement: Capability isolation by role binary

Each role SHALL run a separate binary so that capability is enforced
structurally, not by a runtime role check. The reviewer binary SHALL NOT register
mutating commands (`submit`, `done`, `pr create`); only the role's own binary
SHALL be mounted into its container; and the reviewer's workspace SHALL be
mounted read-only. Both binaries MAY build from shared source, but the command
subset each registers defines the boundary.

#### Scenario: Reviewer cannot mutate code

- **WHEN** the reviewer agent attempts a mutating command
- **THEN** the command does not exist in its binary and its workspace is read-only

## MODIFIED Requirements

### Requirement: One sandboxed container per worker

Each agent SHALL run as a Claude Code agent inside its own Podman container, with
its mount topology determined by its role's `RoleSpec`. A `worker` SHALL get its
own git worktree mounted writable with the main repository read-only; a
`reviewer` SHALL get the main repository mounted read-only as its workspace. In
all cases the agent edits only what its role permits.

#### Scenario: Starting a worker

- **WHEN** a worker is started
- **THEN** a container launches on the worker's worktree with the repo read-only

#### Scenario: Starting a reviewer

- **WHEN** a reviewer is started
- **THEN** a container launches with the whole repository mounted read-only as its
  workspace

### Requirement: Norse-named workers

Workers SHALL be identified by Norse dwarf names (brokkr, dvalin, …). The
reviewer is an agent in the `reviewer` role, distinct from the dwarf workers; it
SHALL NOT take a dwarf name and SHALL NOT be identified by position (e.g. "the
main repo") but by its role in the index.

#### Scenario: Reusing a name

- **WHEN** an idle worker worktree exists
- **THEN** it is reused rather than allocating a new dwarf name

#### Scenario: Reviewer identified by role

- **WHEN** the roster is read
- **THEN** the reviewer is the agent whose index role is `reviewer`, not whichever
  agent occupies the main repo
