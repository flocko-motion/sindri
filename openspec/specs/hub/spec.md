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

### Requirement: One hub per repo is the single writer

Sindri SHALL run at most one hub process per repository, bound to a unix-domain
socket. The hub SHALL be the only process that invokes the td and openspec
adapters and the only writer of git, the PR state, and `.sindri/`. Every other
actor — agents, the host CLI, the TUI — SHALL reach that state only by calling
the hub. Because there is exactly one writer, concurrent mutations SHALL NOT race.

#### Scenario: Logic mutates state through the hub

- **WHEN** any actor changes task, PR, or roster state
- **THEN** the change is performed by the hub process, not by the caller touching
  td/git/the store directly

#### Scenario: Singleton enforced by the socket

- **WHEN** a second hub tries to bind the same repo socket
- **THEN** the bind fails and the second process attaches to the running hub
  instead of starting a rival writer

### Requirement: The socket is the caller's identity

The hub SHALL derive a caller's identity from the socket the connection arrived
on, never from a value the client supplies. Each agent pod SHALL mount exactly
one socket — its own. An agent therefore SHALL NOT be able to name another agent,
enumerate the roster, or address another pod.

#### Scenario: Agent identified by its socket

- **WHEN** an agent calls the hub over its mounted socket
- **THEN** the hub knows which agent is calling without the agent sending a name

#### Scenario: No cross-agent reach

- **WHEN** an agent attempts to act as or address a different agent
- **THEN** it cannot, because its only channel is its own socket and the roster is
  invisible to it

### Requirement: Hub lifecycle — ephemeral for CLI, persistent for agents

A host CLI command SHALL spawn an ephemeral hub when none is running, serve the
command, and exit. A hub with one or more live agents SHALL persist for as long
as those agents exist. When an agent's hub is not running, the agent's call SHALL
fail loudly and instruct the user to start `sindri hub`; the TUI SHALL refuse to
start without a running hub.

#### Scenario: CLI with no hub

- **WHEN** a user runs a `sindri` command and no hub is running
- **THEN** an ephemeral hub is started, the command is served, and it exits

#### Scenario: Agents keep the hub alive

- **WHEN** agents are running
- **THEN** the hub persists rather than exiting after a single CLI command

#### Scenario: Hub down under an agent

- **WHEN** an agent calls a hub that is not running
- **THEN** the call fails with a clear message telling the user to start the hub

### Requirement: Protocol is HTTP/JSON over the unix socket

The hub SHALL serve an HTTP API over its unix socket with JSON bodies. It SHALL
expose: an execute endpoint that streams a command's stdout/stderr and exit code,
a commands endpoint returning the currently available surface for the caller, a
state endpoint returning the board as JSON, and an events endpoint streaming state
changes. Closing the connection (e.g. a pod dying) SHALL cancel the in-flight
request via its context, with no separate keepalive machinery.

#### Scenario: Execute streams output

- **WHEN** a client posts an argv to the execute endpoint
- **THEN** the command's stdout/stderr stream back and the call ends with its exit
  code

#### Scenario: Connection drop cancels work

- **WHEN** a calling pod dies mid-request
- **THEN** the dropped connection cancels the handler's context and the hub
  releases any work tied to it

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

### Requirement: Hub state is durable and crash-restartable

The hub MAY crash or restart at any time and SHALL lose nothing committed. Every
state change SHALL be persisted under `.sindri/` as part of the operation, so the
hub's in-memory state is a rebuildable projection, never the sole source of truth.
A restarted hub SHALL reconstruct full operating state from `.sindri/` plus live
pod inspection. Agent pods and their tmux sessions SHALL run independently of the
hub and survive its restart.

#### Scenario: Crash loses nothing committed

- **WHEN** the hub crashes and is restarted
- **THEN** it reloads the roster and live workflow state from `.sindri/` and
  resumes, with no committed state lost

#### Scenario: Agents survive the blink

- **WHEN** the hub restarts while agents are running
- **THEN** the agent pods and tmux sessions are untouched, and the restarted hub
  re-resolves them and resumes injecting

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
