# Workers — delta

## ADDED Requirements

### Requirement: Agent index is the source of truth

Sindri SHALL maintain a durable index of the agents that exist for a project, as
one JSON file per agent under `.sindri/agents/<name>.json` in the main repo. The
index SHALL record identity only — at least `name`, `role`, `mode`, `base`, the
agent's `workspace` path (which MAY be empty), and a creation timestamp. The
index SHALL be written by the host at launch and SHALL NOT be inferred from
container or worktree position. The `.sindri/` directory SHALL be gitignored
(agents are local).

#### Scenario: Launching records identity

- **WHEN** an agent is launched
- **THEN** its `.sindri/agents/<name>.json` entry exists with its role, mode, and
  base, independent of whether its workspace exists

#### Scenario: Identity survives workspace loss

- **WHEN** an agent's workspace directory is missing
- **THEN** its index entry still describes the agent (role, mode, base) so it can
  be rebuilt from the roster

### Requirement: `.sindri` project scaffold via `sindri init`

Sindri SHALL provide an interactive `sindri init` command that creates the
`.sindri/` scaffold, writes a project config, and ensures `.sindri/` is
gitignored. The command SHALL be idempotent. The TUI SHALL ensure the scaffold
exists at startup, running the init routine before launching when `.sindri/` is
absent.

#### Scenario: Fresh project

- **WHEN** `sindri init` runs in a project with no `.sindri/`
- **THEN** `.sindri/` and its config are created and `.sindri/` is added to
  `.gitignore`

#### Scenario: TUI on an uninitialised project

- **WHEN** the TUI starts and `.sindri/` does not exist
- **THEN** the init routine runs before the TUI is shown

### Requirement: Orphaned agents can be pruned

An orphan SHALL be defined as a podman container or a worktree that exists with
no matching entry in the agent index. Sindri SHALL be able to scan for orphans
and delete them — removing the container and/or the worktree. Because orphans
are most likely stale, deletion SHALL proceed only after explicit user
confirmation.

#### Scenario: Pruning an orphan

- **WHEN** a container or worktree exists with no index entry and the user
  confirms deletion
- **THEN** its container is removed and its worktree is deleted

#### Scenario: Confirmation required

- **WHEN** orphans are found
- **THEN** they are listed and nothing is deleted until the user confirms

### Requirement: Two-layer agent state

Agent identity SHALL live in the index (main repo) and agent live progress
(`task`, `status`) SHALL live in the agent's own workspace (the existing
`.sindri-*` files). The two layers SHALL NOT duplicate each other: the index
points at a workspace; the workspace holds what is happening in it.

#### Scenario: Progress stays in the workspace

- **WHEN** an agent claims a task
- **THEN** the task is recorded in its workspace state file, not copied into the
  index entry

## MODIFIED Requirements

### Requirement: Worker-to-task mapping

A worker's task and status SHALL be determined by reconciling the agent index
(the roll call of agents that should exist) against observed reality (podman
container state, the worktree branch, the workspace task file, and the PR store).
The mapping SHALL start from the index, not from whichever containers or
worktrees happen to exist, and SHALL NOT be inferred by position or guesswork.

#### Scenario: Showing what a worker does

- **WHEN** the board is refreshed
- **THEN** each indexed agent is matched to its task by joining its workspace
  state and worktree branch, and any container/worktree with no index entry is
  reported as an orphan

### Requirement: Fail loudly, heal on pickup

Worker startup problems SHALL surface rather than be silently swallowed. When an
agent picks up the next task, it SHALL first clear any task it left stuck
in-progress from a previous run. Reconciliation SHALL distinguish a crashed
agent (indexed, has a task, no running container) from an idle one (indexed, no
task) and from one whose workspace is missing (indexed, no workspace).

#### Scenario: Orphaned in-progress task

- **WHEN** an agent begins `sindri-worker issue next`
- **THEN** any task it left in-progress is returned to open before claiming a new one

#### Scenario: Crashed mid-task is distinguishable

- **WHEN** an indexed agent has a workspace task but no running container
- **THEN** its status reconciles to "crashed mid-task", distinct from "idle"
