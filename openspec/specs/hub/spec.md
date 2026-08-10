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
from their sources of truth. Tasks MAY come from more than one source — the task
backend, openspec changes, and GitHub issues — merged into the one cache; each
row's id prefix (`td-`, `os-`, `gh-`) records which source owns it. Browsing reads
— lists and the board — SHALL be served from the cache. To bound staleness where
it would mislead or cause a wrong decision, the hub SHALL refresh from the source
of truth: **all tasks at startup**; **a task immediately before it is assigned**
to an agent; and **a task immediately before its detail is shown**. Periodic
background sync and explicit user refresh MAY additionally run. A **network-backed
source** (e.g. GitHub issues) SHALL be throttled — served from a short-lived cache
so the frequent idle-worker resync does not exceed the remote's rate limits — and
SHALL degrade to contributing no tasks when it is unavailable, without failing the
sync of the other sources. Every write SHALL go to the source of truth through
that source's tool, and the hub SHALL update the cache to reflect it.

#### Scenario: Browsing served from cache

- **WHEN** the board or a UI lists tasks
- **THEN** they are read from `hub.db`, not by querying the backend per query

#### Scenario: Refresh all at startup

- **WHEN** the hub starts
- **THEN** it refreshes every task from the sources of truth into `hub.db`

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

#### Scenario: Network source is throttled

- **WHEN** many resyncs occur in quick succession (e.g. an idle worker polling
  every few seconds) with the GitHub source enabled
- **THEN** the GitHub listing is served from a short-lived cache rather than hitting
  the remote on every resync

#### Scenario: One source unavailable, others still sync

- **WHEN** the GitHub source is unavailable during a sync
- **THEN** td and openspec tasks still sync and the cache updates; the GitHub source
  simply contributes no tasks

### Requirement: Orphans are runtime the roster does not account for

The roster in `hub.db` SHALL be the declaration of which agents exist; reality SHALL
be checked against it, not the other way round. A pod or worktree running with no
matching roster entry SHALL be reported as an orphan. The hub SHALL NOT kill an orphan
on its own initiative — no sweep, no reaping, nothing dies unasked. It SHALL surface the
orphan as a warning and SHALL offer removal as an explicit user-initiated action, which
every front end can invoke; the front end SHALL confirm before it is carried out. The
mechanism is the hub's own, not a container-engine command the user is asked to run.

#### Scenario: Orphan detected

- **WHEN** a pod is running with no matching roster entry
- **THEN** it is reported as an orphan the user may remove, and nothing is killed
  automatically

#### Scenario: Orphan removed on request

- **WHEN** a user confirms removal of a reported orphan from either front end
- **THEN** the hub removes that runtime, and no roster entry is touched — there was none

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
action is invisible rather than rejected. The hub SHALL recognise four roles —
worker, reviewer, planner, and coauthor — and the surface SHALL be scoped to each:
a worker registers and inspects merge-intents (`next`/`submit`); a reviewer judges
them (`approve`/`reject`/`review`); a planner reads the backlog and proposes work
(`task`/`create-task`/`openspec`); a coauthor gets the generic helpers only, since
it commits with git directly. No role SHALL ever see merge.

#### Scenario: Blocked-on-PR worker

- **WHEN** a worker has a branch awaiting a merge verdict
- **THEN** "pick up the next task" is absent from its command surface until the
  verdict arrives

#### Scenario: Reviewer never sees submit

- **WHEN** a reviewer queries its command surface
- **THEN** worker-only verbs such as submit are absent from it

#### Scenario: Planner surface is propose-and-ship

- **WHEN** a planner queries its command surface
- **THEN** it sees `task`, `create-task`, and `openspec` but never the worker's
  `next`/`submit` nor the reviewer's `approve`/`reject`

#### Scenario: Coauthor surface is helpers only

- **WHEN** a coauthor queries its command surface
- **THEN** it sees only the generic helpers (status, log, lint, read-only PR views)
  and none of the worker, reviewer, or planner workflow verbs

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

#### Scenario: Task data is never committed

- **WHEN** the hub first serves a repo
- **THEN** it ensures the repo's `.gitignore` lists both `.worktrees/` and
  `.todos/`, so the constantly-rewritten task DB can never be committed and collide
  with the host checkout's live `.todos/` at merge time

### Requirement: Worker can re-test mergeability on demand

A worker SHALL be able to ask the hub to bring its branch up to its base, as often as it wants, and learn the result. The hub reports one of: already current, rebased cleanly, or conflicted — and when conflicted, which files conflict. This lets a worker iterate toward a mergeable branch instead of discovering the problem only at the human merge.

#### Scenario: Branch is behind but clean

- **WHEN** a worker asks the hub to test mergeability and the branch merely trails base
- **THEN** the hub rebases it onto base and reports success, with no conflict to resolve

#### Scenario: Branch conflicts with base

- **WHEN** a worker asks the hub to test mergeability and the rebase conflicts
- **THEN** the hub reports the conflict and names the files that need resolving

#### Scenario: Worker re-asks after editing

- **WHEN** a worker asks again after editing the conflicted files
- **THEN** the hub resumes the git operation from where it stopped, not from scratch

### Requirement: Hub performs all git; the worker only resolves content

The hub SHALL perform every git operation (rebase, stage, continue, commit) host-side, because the worker has no git access — only its worktree files are mounted. On a conflict the hub SHALL surface the conflict into the worker's worktree (leave the conflict markers in place, not abort), so the worker resolves the file *content*. The worker SHALL never be asked to run git or to perform host/operator setup.

#### Scenario: Conflict is left in the worktree to resolve

- **WHEN** the hub's rebase of a worker's branch conflicts
- **THEN** the conflicted files remain in the worker's worktree with conflict markers, and the rebase is left in progress rather than aborted

#### Scenario: Hub advances the rebase after the worker resolves

- **WHEN** the worker has removed the conflict markers and asks the hub to continue
- **THEN** the hub stages the resolved files and continues the rebase host-side, surfacing the next conflict or reporting completion

#### Scenario: Worker is never told to run git or infra commands

- **WHEN** the hub reports a conflict to a worker
- **THEN** the message describes which files to edit, and never instructs the worker to run git or any host/operator command

### Requirement: A branch reaches the human merge only when it applies cleanly

The hub SHALL NOT hand a branch that conflicts with its base to the human merge. A conflict discovered at merge time SHALL route the branch into the worker-driven resolution loop rather than a dead-end rejection, and once the branch applies cleanly onto base the local PR SHALL be renewed for re-review so the human merge is conflict-free.

#### Scenario: Merge-time conflict enters the resolution loop

- **WHEN** a human triggers a merge and the branch conflicts with base
- **THEN** the branch is routed to its worker for content resolution (the resolution loop), not rejected as "resubmit and try again"

#### Scenario: Clean branch renews the PR for review

- **WHEN** a worker's branch has been rebased cleanly onto base
- **THEN** the local PR is renewed and re-offered for review, after which the human merge applies without conflict

### Requirement: Planner task proposals are gated on user approval

A planner SHALL propose backlog tasks with `create-task`, but a proposed task
SHALL NOT be claimable by any worker until the user approves it. The hub SHALL
record a per-task approval state — pending, approved, or rejected — held in
`hub.db` separate from the task's own status. A task with no approval row is a
normal, claimable task; a task flagged pending or rejected SHALL be hidden from
the work an agent can claim. Approval and rejection SHALL be user-only actions
(`sindri task approve`/`reject`), and the hub SHALL inject the verdict into every
running planner's session.

#### Scenario: Proposed task is withheld until approved

- **WHEN** a planner runs `create-task`
- **THEN** the task is created in the backend flagged pending the user's approval,
  and no worker can claim it while it is pending

#### Scenario: User approves a proposal

- **WHEN** the user approves a planner-proposed task
- **THEN** the approval gate clears, the task becomes claimable by a worker, and
  any running planner is told it was approved

#### Scenario: User rejects a proposal

- **WHEN** the user rejects a planner-proposed task with a comment
- **THEN** the task stays hidden from workers and the comment is injected into any
  running planner's session

