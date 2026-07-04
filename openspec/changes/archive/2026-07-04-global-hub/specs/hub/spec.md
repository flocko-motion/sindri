## RENAMED Requirements

- FROM: `### Requirement: One hub per repo is the single writer`
- TO: `### Requirement: One global hub is the single writer across repos`

- FROM: `### Requirement: The socket is the caller's identity`
- TO: `### Requirement: Identity is the (project, agent) pair, never client-supplied`

- FROM: `### Requirement: Hub lifecycle — ephemeral for CLI, persistent for agents`
- TO: `### Requirement: Hub lifecycle — one persistent global daemon`

- FROM: `### Requirement: Protocol is HTTP/JSON over the unix socket`
- TO: `### Requirement: Protocol is HTTP/JSON carrying repo context`

- FROM: `### Requirement: Hub state is durable and crash-restartable`
- TO: `### Requirement: Hub state is durable, central, and crash-restartable`

## MODIFIED Requirements

### Requirement: One global hub is the single writer across repos

Sindri SHALL run at most one hub process for the whole machine, bound to a single
control socket, serving every repository. The hub SHALL be the only process that
invokes the td and openspec adapters and the only writer of git, PR state, and its
central state directory. Every other actor — agents, the host CLI, the TUI — SHALL
reach that state only by calling the hub, passing the repo (project) each request
concerns. Because there is exactly one writer across all repos, concurrent
mutations SHALL NOT race.

#### Scenario: Logic mutates state through the hub

- **WHEN** any actor changes task, PR, or roster state for a repo
- **THEN** the change is performed by the single hub process for that project, not
  by the caller touching td/git/the store directly

#### Scenario: Singleton enforced by the socket

- **WHEN** a second hub tries to bind the control socket
- **THEN** the bind fails and the second process attaches to the running hub
  instead of starting a rival writer

### Requirement: Identity is the (project, agent) pair, never client-supplied

The hub SHALL derive a caller's identity from the channel the connection arrived
on, never from a value the client freely supplies. On Linux this is the agent's own
mounted socket; on macOS (where a bind-mounted socket cannot cross the podman VM) it
is a per-agent bearer token. Either way the hub SHALL resolve the caller to a
specific `(project, agent)`, and an agent SHALL NOT be able to name another agent,
enumerate any roster, or address another pod — in its own project or any other.

#### Scenario: Agent identified by its channel

- **WHEN** an agent calls the hub over its socket (Linux) or with its token (macOS)
- **THEN** the hub knows which `(project, agent)` is calling without the agent
  sending a name

#### Scenario: No cross-agent or cross-project reach

- **WHEN** an agent attempts to act as or address a different agent, or reach
  another project
- **THEN** it cannot, because its channel resolves only to its own `(project, agent)`
  and no roster is visible to it

### Requirement: Hub lifecycle — one persistent global daemon

The hub SHALL be a single long-lived daemon serving all repos. Interactive entry
points (`sindri coauthor`, `sindri tui`) SHALL auto-start it in the background when
none is running; `sindri hub start` runs it explicitly (foreground, or `--bg`).
Once running it SHALL persist across individual CLI commands and for as long as any
agent in any repo exists. When the hub is not running, an agent's call SHALL fail
loudly.

#### Scenario: Interactive command with no hub

- **WHEN** a user runs `sindri coauthor` or `sindri tui` and no hub is running
- **THEN** a background hub is started, then the command proceeds

#### Scenario: Agents keep the hub alive

- **WHEN** agents are running in any repo
- **THEN** the single hub persists rather than exiting

### Requirement: Protocol is HTTP/JSON carrying repo context

The hub SHALL serve an HTTP API with JSON bodies over its control socket. Every
request that concerns a specific repository SHALL carry that repo's context (its
root), and the hub SHALL scope reads/writes to that project. It SHALL expose: an
execute endpoint that streams a command's stdout/stderr and exit code, a commands
endpoint returning the caller's available surface, a state endpoint returning the
board (agents and PRs across all projects, tasks for the requested project), and an
events endpoint streaming state changes. Closing the connection SHALL cancel the
in-flight request via its context.

#### Scenario: Request scoped by project

- **WHEN** a client posts a repo-scoped command with its repo context
- **THEN** the hub applies it to that project's state and no other

#### Scenario: Connection drop cancels work

- **WHEN** a calling pod dies mid-request
- **THEN** the dropped connection cancels the handler's context and the hub
  releases any work tied to it

### Requirement: Hub state is durable, central, and crash-restartable

The hub MAY crash or restart at any time and SHALL lose nothing committed. All hub
state SHALL be persisted centrally under a machine-level state directory
(`$XDG_STATE_HOME/sindri`, overridable via `SINDRI_HOME`) — never inside any repo —
in one project-keyed store, so the hub's in-memory state is a rebuildable
projection. A restarted hub SHALL reconstruct full operating state from that store
plus live pod inspection, for every project. Agent pods and their tmux sessions
SHALL run independently of the hub and survive its restart.

#### Scenario: Crash loses nothing committed

- **WHEN** the hub crashes and is restarted
- **THEN** it reloads every project's roster and workflow state from the central
  store and resumes, with no committed state lost

#### Scenario: Agents survive the blink

- **WHEN** the hub restarts while agents are running
- **THEN** the agent pods and tmux sessions are untouched, and the restarted hub
  re-resolves them and resumes injecting

## ADDED Requirements

### Requirement: One project-keyed store, no per-repo state files

The hub SHALL keep all its state in a single store under the central state dir,
with every per-repo row tagged by a project key (`repoTag`, a stable digest of the
repo's absolute path). Agent identity SHALL be unique per `(project, name)`, so the
same agent name MAY exist in different repos. The hub SHALL NOT write any state into
the repositories it serves; a repo's only sindri-related on-disk content is
git-owned worktrees and td's own `.todos/`.

#### Scenario: Same agent name in two repos

- **WHEN** two different repos each register an agent named "eitri"
- **THEN** both exist as distinct `(project, name)` identities and never collide

#### Scenario: Repo stays free of hub state

- **WHEN** the hub serves a repo
- **THEN** it writes no `.sindri/` into that repo; all hub state lives under the
  central state dir
