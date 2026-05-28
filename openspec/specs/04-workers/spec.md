# Workers

## Purpose

Defines sindri's workers: sandboxed AI agents (the dwarves) that pick up tasks
and produce PRs. Each worker is a Claude Code agent running in its own Podman
container against its own git worktree. This spec covers the container/worktree
lifecycle and how a worker maps to the board; the agent's task loop uses the
gh-local workflow.

## Requirements

### Requirement: One sandboxed container per worker

Each worker SHALL run as a Claude Code agent inside its own Podman container,
bound to its own git worktree. The main repository SHALL be mounted read-only
(source) with the shared git metadata writable; the worker edits only its
worktree.

#### Scenario: Starting a worker

- **WHEN** a worker is started
- **THEN** a container launches on the worker's worktree with `/repo` read-only

### Requirement: Norse-named workers

Workers SHALL be identified by Norse dwarf names (brokkr, dvalin, …). The review
agent SHALL be distinct from the dwarf workers and SHALL not take a dwarf name.

#### Scenario: Reusing a name

- **WHEN** an idle worker worktree exists
- **THEN** it is reused rather than allocating a new dwarf name

### Requirement: Worker-to-task mapping

A worker's current task SHALL be discoverable from the host by reading the
worktree's branch (a `td-……` name) and/or a task state file the agent writes.
The mapping SHALL NOT be inferred by position or guesswork.

#### Scenario: Showing what a worker does

- **WHEN** the board is refreshed
- **THEN** each running worker is matched to its task via its worktree branch/state

### Requirement: Fail loudly, heal on pickup

Worker startup problems SHALL surface rather than be silently swallowed. When an
agent picks up the next task, it SHALL first clear any task it left stuck
in-progress from a previous run.

#### Scenario: Orphaned in-progress task

- **WHEN** an agent begins `sindri-worker issue next`
- **THEN** any task it left in-progress is returned to open before claiming a new one

### Requirement: Bundled agent tooling

The container image SHALL bundle the tools an agent needs — the `sindri-worker`
workflow CLI, `td`, and the openspec CLI — so a worker can drive the full propose
→ implement → submit → review loop offline. The `sindri-worker` binary SHALL be
mounted from the host so it can be updated without rebuilding the image.

#### Scenario: Agent runs the loop

- **WHEN** a worker container starts
- **THEN** `sindri-worker`, `td`, and `openspec` are all on its PATH

## Structure

- `internal/worker/` (`type: adapter`) — worker discovery/status
  (`worker.go`) and the container/worktree lifecycle (`lifecycle.go`).
- `container/` — the agent image (Dockerfile), skills, and CLAUDE.md.
- `cmd/sindri/` (`type: command`) — `worker`/`work`/`review` wiring.
