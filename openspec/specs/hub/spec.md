# Hub

## Purpose

Defines the hub: the single per-repo writer process that owns all sindri state.
The hub is the only actor that invokes the td and openspec adapters and the only
writer of git, PR state, and `.sindri/`. Every other actor — agents, the host
CLI, the TUI — reaches that state only by calling the hub over a unix socket.
This capability covers the hub's singleton lifecycle, its socket-derived
identity model, its HTTP/JSON protocol, its durable crash-restartable state, the
cached task read model, the per-agent activity log, and the state-filtered
command surface.
## Requirements
### Requirement: Abstract tasks are a cached read model

The hub SHALL hold abstract tasks in `hub.db` as a fast local read model, synced
from their source of truth (the task backend). Browsing reads — lists and the board
— SHALL be served from the cache. To bound staleness where it would mislead or cause
a wrong decision, the hub SHALL refresh from the source of truth: **all tasks at
startup**; **a task immediately before it is assigned** to an agent; and **a task
immediately before its detail is shown**. Periodic background sync and explicit user
refresh MAY additionally run. Every write SHALL go to the source of truth through the
backend's tool, and the hub SHALL update the cache to reflect it.

#### Scenario: Browsing served from cache

- **WHEN** the board or a UI lists tasks
- **THEN** they are read from `hub.db`, not by querying the backend per query

#### Scenario: Refresh all at startup

- **WHEN** the hub starts
- **THEN** it refreshes every task from the source of truth into `hub.db`

#### Scenario: Refresh before assignment

- **WHEN** a task is about to be assigned to an agent
- **THEN** the hub refreshes that task from the source of truth first, so an already
  changed or closed task is never handed out

#### Scenario: Refresh before detail

- **WHEN** a task's detail is shown
- **THEN** the hub refreshes that task from the source of truth before presenting it

#### Scenario: Write reaches the source of truth

- **WHEN** a task is created or changed
- **THEN** the change is written through the backend's tool and the cached copy is
  updated to match

### Requirement: Orphans are runtime the roster does not account for

The roster in `hub.db` SHALL be the declaration of which agents exist; reality SHALL
be checked against it, not the other way round. A pod or worktree running with no
matching roster entry SHALL be reported as an orphan. The hub SHALL NOT silently
kill orphans; it SHALL surface them as a warning and propose a shell command the
user can run to remove them.

#### Scenario: Orphan detected

- **WHEN** a pod is running with no matching roster entry
- **THEN** it is reported as an orphan with a proposed removal command, and nothing
  is killed automatically

#### Scenario: Declared agent with no pod is not an orphan

- **WHEN** an agent is in the roster but has no running pod
- **THEN** it is a stopped, launchable agent — not an orphan

### Requirement: Agents exist independently of pods; launch binds and rehydrates

An agent SHALL exist as a durable roster entry independent of any running pod — it
MAY exist with no pod (pre-declared, stopped, or crashed). The hub SHALL be able to
launch a pod for an existing agent; that pod SHALL assume the agent's identity via
its mounted socket. On launch or relaunch, the hub SHALL be able to rehydrate the
agent by injecting a briefing drawn from the tail of its activity log, so a fresh
session resumes the agent's prior work.

#### Scenario: Agent without a pod

- **WHEN** an agent is registered but no pod is running
- **THEN** it still exists in the roster and can be launched later

#### Scenario: Launch assumes the identity

- **WHEN** the hub launches a pod for an existing agent
- **THEN** the pod takes that agent's identity through its mounted socket

#### Scenario: Resume from history

- **WHEN** an agent is launched or relaunched after its previous pod ended
- **THEN** the hub injects a briefing from the tail of its activity log so it knows
  what it was doing

### Requirement: Per-agent activity is logged durably

The hub SHALL record an append-only activity log per agent, persisted in
`.sindri/hub.db`. The log SHALL capture all hub-mediated interaction: the commands
an agent runs over the socket and their results, every message the hub injects
(with its provenance tag), merge-intent registrations and verdicts, and status
transitions. The log SHALL NOT include the agent's freeform terminal chat, which
is observed separately. The log SHALL survive hub restarts.

#### Scenario: A socket command is logged

- **WHEN** an agent runs a command over the socket
- **THEN** the hub appends an entry recording the command and its result

#### Scenario: An injected message is logged

- **WHEN** the hub injects a message into an agent's session
- **THEN** the hub appends an entry recording the message and its provenance

#### Scenario: Freeform chat excluded

- **WHEN** the agent produces freeform reasoning/output in its pane
- **THEN** that content is not written to the activity log; it is observed via
  attach or capture instead

### Requirement: Command surface is state-filtered

The hub SHALL compute the set of commands available to a caller from its role and
current state, and the commands endpoint SHALL return only what is possible right
now. A command that is not currently valid SHALL NOT appear, so an out-of-order
action is invisible rather than rejected.

#### Scenario: Blocked-on-PR worker

- **WHEN** a worker has a branch awaiting a merge verdict
- **THEN** "pick up the next task" is absent from its command surface until the
  verdict arrives

#### Scenario: Reviewer never sees submit

- **WHEN** a reviewer queries its command surface
- **THEN** worker-only verbs such as submit are absent from it

### Requirement: Sections with actionable counts

The hub SHALL expose a section model — an ordered set of sections, each with a
key, a title, and a count derived from board state — as the single source of
truth for which views exist and the badge each shows. The counts SHALL be the
actionable subset: non-closed tasks, running agents, and not-merged PRs. UIs
SHALL render these counts rather than computing their own.

#### Scenario: A UI renders section counts

- **WHEN** a UI draws its tabs
- **THEN** each tab's badge is the hub-provided count for that section

#### Scenario: Adding a section

- **WHEN** a new section is introduced
- **THEN** it is added to the hub's section model and UIs pick it up without
  re-deriving counts

### Requirement: Task hierarchy arrangement

The hub SHALL arrange a flat set of tasks into their parent/child tree — roots
ordered by priority, each followed by its descendants, with a depth per node —
and annotate each with the id of a non-merged PR for that task, if any. A task
whose parent is absent from the set SHALL be arranged as a root. This arrangement
SHALL be a logic-layer function so every UI renders the same tree.

#### Scenario: Tree with depth

- **WHEN** tasks with parent relationships are arranged
- **THEN** the result lists each parent before its children with an increasing
  depth, and standalone tasks at depth zero

#### Scenario: PR annotation

- **WHEN** a task has a non-merged PR
- **THEN** its arranged row carries that PR's id

### Requirement: Board carries all tasks with hierarchy

The board state the hub serves SHALL include all tasks (every status), each with
its parent and a description, so a UI can show what is being worked — by whom, in
its hierarchy — and can filter to open/closed/all client-side. Section counts
SHALL derive the non-closed subset from this full set.

#### Scenario: In-progress and closed tasks both present

- **WHEN** the board is requested
- **THEN** it includes in_progress tasks (with parent + assignable detail) and
  closed tasks, so a UI can filter between them without another fetch

### Requirement: PR detail includes its linked task

A PR's detail from the hub SHALL include the linked task (id, title, status) in
addition to the diff, resolved from the source of truth so it is present even
after the task closes on merge.

#### Scenario: PR detail carries the task

- **WHEN** a PR's detail is requested
- **THEN** it includes the linked task's id, title, and status, and the diff

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

### Requirement: One project-keyed store, no per-repo state files

The hub SHALL keep all its state in a single store under the central state dir,
with every per-repo row tagged by a project key (`repoTag`, a stable digest of the
repo's absolute path). Agent identity SHALL be unique per `(project, name)`, so the
same agent name MAY exist in different repos. The hub SHALL NOT write any state into
the repositories it serves; a repo's only sindri-related on-disk content is
git-owned worktrees and td's own `.todos/` — both gitignored by the hub, never
committed.

#### Scenario: Same agent name in two repos

- **WHEN** two different repos each register an agent named "eitri"
- **THEN** both exist as distinct `(project, name)` identities and never collide

#### Scenario: Repo stays free of hub state

- **WHEN** the hub serves a repo
- **THEN** it writes no `.sindri/` into that repo; all hub state lives under the
  central state dir

