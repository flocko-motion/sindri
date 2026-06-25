# Workers — delta

## MODIFIED Requirements

### Requirement: One sandboxed container per worker

Each agent SHALL run as a Claude agent inside its own Podman pod, launched by the
hub, mounting its workspace and exactly one unix socket — its own channel to the
hub. The agent SHALL NOT have the task database, the roster, or other agents'
workspaces mounted; all such state is reached only through the hub over the
socket. A worker or reviewer SHALL mount an isolated per-agent git worktree
(named after the agent) read-write. A coauthor SHALL differ: because it works the
same material as the user, its workspace SHALL be the user's own repository
checkout (the repo root) mounted read-write, with the `.sindri/` directory
overlaid by an empty read-only directory so the agent can neither read nor corrupt
hub state in the shared tree.

#### Scenario: Starting an agent

- **WHEN** the hub launches an agent
- **THEN** a pod starts with the agent's workspace and its single hub socket
  mounted, and the task database and roster are not

#### Scenario: Same mounts for worker and reviewer

- **WHEN** a worker and a reviewer are launched
- **THEN** their mounts are identical — an isolated read-write worktree plus the
  hub socket; only the hub-side role differs

#### Scenario: Coauthor shares the user's checkout with .sindri shielded

- **WHEN** a coauthor is launched
- **THEN** its `/workspace` is the user's repository checkout mounted read-write,
  and `.sindri/` is overlaid by an empty read-only directory so hub state is
  neither readable nor writable from the shared tree

### Requirement: Norse-named workers

Agents SHALL be auto-named from a pool of Norse dwarf names (dvalin, eitri, …) —
the smith binaries' own names are never handed out. The pool is role-agnostic: a
worker, reviewer, planner, or coauthor each receives the next unused dwarf name
unless an explicit name is supplied at registration.

#### Scenario: Auto-named regardless of role

- **WHEN** an agent of any role is registered without an explicit name
- **THEN** it receives the first unused dwarf name, whatever its role

#### Scenario: Reusing a name

- **WHEN** an idle worker worktree exists
- **THEN** it is reused rather than allocating a new dwarf name

## ADDED Requirements

### Requirement: Agents share the user's Claude skills

When the hub launches an agent that runs Claude, it SHALL make the user's Claude
skills available inside the agent's Claude home by mounting the host's skills
directory read-only, so the agent works with the same skills the user has. The
mount SHALL be live — edits to a skill on the host are reflected inside the pod
without relaunching the agent — and read-only, so the agent cannot alter the
user's skills. When the host has no skills directory, the launch SHALL proceed
without it rather than failing.

#### Scenario: Agent has the user's skills

- **WHEN** an agent that runs Claude is launched and the user has a skills directory
- **THEN** those skills are present in the agent's Claude home, read-only

#### Scenario: Skill edits are live

- **WHEN** the user edits a skill on the host while an agent is running
- **THEN** the agent sees the updated skill without being relaunched

#### Scenario: No skills directory

- **WHEN** an agent is launched and the host has no Claude skills directory
- **THEN** the launch proceeds normally, simply without any mounted skills
