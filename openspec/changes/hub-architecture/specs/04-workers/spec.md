# Workers — delta

## MODIFIED Requirements

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

### Requirement: Bundled agent tooling

The pod image SHALL bundle only what an agent needs to run and talk to the hub:
the single role-agnostic agent client and tmux. The client SHALL carry no
built-in command tree; its available commands come from the hub. There SHALL NOT
be separate worker and reviewer binaries.

#### Scenario: Agent talks to the hub

- **WHEN** an agent pod starts
- **THEN** the role-agnostic client and tmux are present, and the client's command
  surface is whatever the hub currently permits
