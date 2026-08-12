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
hub, mounting its workspace and exactly one unix socket — its own channel to the
hub. The agent SHALL NOT have the task database, the roster, or other agents'
workspaces mounted; all such state is reached only through the hub over the
socket. A worker or reviewer SHALL mount an isolated per-agent git worktree
(named after the agent) read-write. A coauthor SHALL differ: because it works the
same material as the user, its workspace SHALL be the user's own repository
checkout (the repo root) mounted read-write, with the `.sindri/` directory
overlaid by an empty read-only directory so the agent can neither read nor corrupt
hub state in the shared tree. A planner SHALL differ again: its workspace SHALL be
mounted read-only with `openspec/` overlaid read-write, so it plans (specs +
tasks) without touching code.

#### Scenario: Starting an agent

- **WHEN** the hub launches an agent
- **THEN** a pod starts with the agent's workspace and its single hub socket
  mounted, and nothing else

#### Scenario: Same mounts for every role

- **WHEN** a worker and a reviewer are launched
- **THEN** their mounts are identical — a read-write workspace plus the hub socket;
  only the hub-side role differs

#### Scenario: Coauthor shares the user's checkout with .sindri shielded

- **WHEN** a coauthor is launched
- **THEN** its `/workspace` is the user's repository checkout mounted read-write,
  and `.sindri/` is overlaid by an empty read-only directory so hub state is
  neither readable nor writable from the shared tree

#### Scenario: Planner workspace is read-only except openspec

- **WHEN** a planner is launched
- **THEN** its `/workspace` is mounted read-only with `openspec/` overlaid
  read-write, so it can edit specs but not the rest of the code

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

### Requirement: An agent may hold a container plus a rolling current subtask

In the container workflow an agent's assignment SHALL be a container task together
with a *current subtask* drawn from the container's open children. The current
subtask rolls forward as each one is checkpointed; the agent SHALL NOT block
between subtasks. The one deliberate pause is a milestone PR, which parks the
agent until the human merges, after which it resumes the same container. The hub
owns both the container assignment and the current subtask as durable state
(recoverable after a restart, like all worker-to-task mapping). When the container
closes, the agent is freed and rejoins normal (leaf) assignment.

#### Scenario: Working state spans subtasks

- **WHEN** an agent finishes one subtask of its container and takes the next
- **THEN** it stays in a working state throughout, never parking in a blocked
  "submitted" state between subtasks

#### Scenario: Recovered after restart

- **WHEN** the hub restarts while an agent holds a container
- **THEN** the container assignment and the current subtask are reloaded from
  durable state, not guessed from branch or worktree position

#### Scenario: Freed on container completion

- **WHEN** an agent's container is closed
- **THEN** the agent is freed and becomes eligible for normal leaf assignment again

### Requirement: No direct task-tracker access in the pod

The agent pod image SHALL NOT include the task-tracker CLI (`td`). An agent SHALL
reach task state — reads and writes alike — only through the hub over its socket;
the hub is the single writer of task state, operating on the main checkout. This
keeps the task tracker's runtime files (`.todos/`) from ever being written in, and
committed from, an agent's worktree, so a PR branch carries only the agent's own
changes.

#### Scenario: No td in the pod

- **WHEN** an agent pod starts
- **THEN** no `td` binary is present, and the agent can read or change task state
  only via the hub over its socket

#### Scenario: Branch stays free of task-tracker churn

- **WHEN** an agent's work is committed
- **THEN** the commit contains only the agent's changes and never `.todos/`
  runtime churn, because nothing in the worktree can write the tracker

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

#### Scenario: Reusing a name

- **WHEN** an idle worker worktree exists
- **THEN** it is reused rather than allocating a new dwarf name

## Structure

- `internal/hub/agent/` (`type: logic`) — the agent lifecycle: launch and stop, the
  worktree and pod per agent, message injection, and the per-agent memory limit.
- `internal/adapter/container/` (`type: adapter`) — the container runtime behind a
  single port, with `pod` (podman) and `applecontainer` backends.
- `internal/container/` (`type: adapter`) — the agent image: the embedded build
  context (Dockerfile, entrypoint, shims) and the build itself.
- `internal/adapter/tmux/` (`type: adapter`) — the session an agent runs in, and how
  the hub types into it.
- `cmd/sindri/` (`type: command`) — the `agent` verbs; the TUI's Agents tab drives the
  same operations through `internal/ui/tui`.

