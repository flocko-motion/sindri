# 04-workers — delta

## MODIFIED Requirements

### Requirement: One sandboxed container per worker

Each agent SHALL run as a Claude agent inside its own Podman pod, launched by the
hub, mounting its workspace and exactly one unix socket — its own channel to the
hub. The agent SHALL NOT have the task database, the roster, or other agents'
workspaces mounted; all such state is reached only through the hub over the
socket. A worker or reviewer SHALL mount an isolated per-agent git worktree
(named after the agent) read-write. A planner SHALL differ: its workspace SHALL be
mounted read-only with `openspec/` overlaid read-write, so it plans (specs +
tasks) without touching code.

A coauthor SHALL differ furthest, and is the deliberate exception to isolation. Because it works the
same material as the user, its `/workspace` SHALL be the user's own repository checkout (the repo root)
mounted read-write. That checkout CONTAINS every other agent's worktree, so the directory holding them
SHALL be covered by an empty read-only directory: the hub commits from those trees, and an edit made
there would land in another agent's pull request as that agent's work. Covering beats mounting them
read-only, because a live worktree is the wrong thing to read as well — it holds work in progress and
build output, which say what that agent is doing now rather than what its pull request contains.

A coauthor SHALL therefore mount a SECOND workspace: a scratch git worktree of its own, read-write,
which the hub checks branches, commits and pull requests out into. It is what replaces the access
being taken away — without it the role could inspect nothing but the user's tree and a diff. It is
disposable by design and is removed with the agent. The pod topology is otherwise unchanged: one
socket, and no state of the hub's own.

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
- **THEN** its `/workspace` is the user's repository checkout mounted read-write, the directory holding
  the other agents' worktrees is covered by an empty read-only directory so it can neither read nor
  write another agent's tree, and there is no hub state in that checkout to shield — the hub's own
  state lives outside every repository (the scenario keeps its title because renaming one drops it)

#### Scenario: Coauthor gets a scratch worktree of its own

- **WHEN** a coauthor is launched
- **THEN** a second worktree is mounted read-write at `/scratch`, and it is removed when the agent is

#### Scenario: Planner workspace is read-only except openspec

- **WHEN** a planner is launched
- **THEN** its `/workspace` is mounted read-only with `openspec/` overlaid
  read-write, so it can edit specs but not the rest of the code
