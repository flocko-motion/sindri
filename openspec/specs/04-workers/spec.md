# Workers

## Purpose

Defines sindri's workers: sandboxed AI agents (the dwarves) that pick up tasks
and produce PRs. Each worker is a Claude Code agent running in its own Podman
container against its own git worktree. This spec covers the container/worktree
lifecycle and how a worker maps to the board; the agent's task loop uses the
gh-local workflow.

## Requirements

### Requirement: One sandboxed container per worker

Each agent SHALL run as a Claude agent inside its own Podman pod, launched by the
hub, with an identical mount topology regardless of role: its git workspace
(named after the agent) and exactly one unix socket — its own channel to the hub.
The agent SHALL NOT have the task database, the roster, or other agents'
workspaces mounted; all such state is reached only through the hub over the
socket.

#### Scenario: Starting an agent

- **WHEN** the hub launches an agent
- **THEN** a pod starts with the agent's workspace and its single hub socket
  mounted, and nothing else

#### Scenario: Same mounts for every role

- **WHEN** a worker and a reviewer are launched
- **THEN** their mounts are identical; only the hub-side role differs

### Requirement: Norse-named workers

Workers SHALL be identified by Norse dwarf names (brokkr, dvalin, …). The review
agent SHALL be distinct from the dwarf workers and SHALL not take a dwarf name.

#### Scenario: Reusing a name

- **WHEN** an idle worker worktree exists
- **THEN** it is reused rather than allocating a new dwarf name

### Requirement: Worker-to-task mapping

An agent's task and status SHALL be owned by the hub and persisted durably under
`.sindri/` as they change. The mapping SHALL NOT be reconstructed by joining or
inferring from container/worktree position. A restarted hub SHALL recover each
agent's task and status from the persisted state; the hub's in-memory copy is a
rebuildable projection of the durable store, never the sole source of truth.

#### Scenario: Showing what an agent does

- **WHEN** the board is queried
- **THEN** each agent's task and status come from the hub's state, not from a
  position-based reconciliation

#### Scenario: Recovered after restart

- **WHEN** the hub restarts
- **THEN** each agent's task and status are reloaded from `.sindri/`, not guessed
  from container or worktree position

### Requirement: Fail loudly, heal on pickup

Worker startup problems SHALL surface rather than be silently swallowed. When an
agent picks up the next task, it SHALL first clear any task it left stuck
in-progress from a previous run.

#### Scenario: Orphaned in-progress task

- **WHEN** an agent begins `sindri-worker issue next`
- **THEN** any task it left in-progress is returned to open before claiming a new one

### Requirement: Bundled agent tooling

The pod image SHALL bundle only what an agent needs to run and talk to the hub:
the single role-agnostic agent client and tmux. The client SHALL carry no
built-in command tree; its available commands come from the hub. There SHALL NOT
be separate worker and reviewer binaries.

#### Scenario: Agent talks to the hub

- **WHEN** an agent pod starts
- **THEN** the role-agnostic client and tmux are present, and the client's command
  surface is whatever the hub currently permits

## Structure

- `internal/worker/` (`type: adapter`) — worker discovery/status
  (`worker.go`) and the container/worktree lifecycle (`lifecycle.go`).
- `container/` — the agent image (Dockerfile), skills, and CLAUDE.md.
- `cmd/sindri/` (`type: command`) — `worker`/`work`/`review` wiring.
