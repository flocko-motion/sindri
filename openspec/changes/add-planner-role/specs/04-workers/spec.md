# Workers — delta

## MODIFIED Requirements

### Requirement: One sandboxed container per worker

Each agent SHALL run as a Claude agent inside its own Podman pod, launched by the
hub, mounting its git workspace (named after the agent) and exactly one unix
socket — its own channel to the hub. The agent SHALL NOT have the task database,
the roster, or other agents' workspaces mounted; all such state is reached only
through the hub over the socket. Workers and reviewers SHALL have an identical
mount topology with a read-write workspace. A planner SHALL differ: its workspace
SHALL be mounted read-only with `openspec/` overlaid read-write, so it plans
(specs + tasks) without touching code.

#### Scenario: Starting an agent

- **WHEN** the hub launches an agent
- **THEN** a pod starts with the agent's workspace and its single hub socket
  mounted, and nothing else

#### Scenario: Same mounts for worker and reviewer

- **WHEN** a worker and a reviewer are launched
- **THEN** their mounts are identical — a read-write workspace plus the hub socket;
  only the hub-side role differs

#### Scenario: Planner workspace is read-only except openspec

- **WHEN** a planner is launched
- **THEN** its `/workspace` is mounted read-only with `openspec/` overlaid
  read-write, so it can edit specs but not the rest of the code

### Requirement: Norse-named agents

Agents SHALL be auto-named from a pool of Norse dwarf names (brokkr, dvalin, …) —
the smith Sindri's own name is never handed out. The pool is role-agnostic: a
worker, reviewer, or planner each receives the next unused dwarf name unless an
explicit name is supplied at registration. When the pool is exhausted a numeric
suffix SHALL be appended (brokkr2, eitri2, …) so creation never fails.

#### Scenario: Auto-named from the pool

- **WHEN** an agent is registered without an explicit name
- **THEN** it receives the first unused dwarf name, regardless of its role

#### Scenario: Pool exhausted

- **WHEN** every dwarf name is already taken
- **THEN** a numeric suffix is appended so a new agent can still be named
