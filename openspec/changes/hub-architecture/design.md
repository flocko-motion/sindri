## Context

Sindri today is a shared-state system: the agent CLI (inside a pod), the user
CLI, and the TUI all write td, git, and the `.git/pr` JSON store directly.
Worker isolation is by instruction only — `container/CLAUDE.md` asks the agent
not to use `td` while the td DB is mounted read-write in the pod. The proposal
inverts this into a hub: one per-repo service that is the sole writer and
gatekeeper, with every other actor a thin client over a unix socket.

Sindri has **no users yet**. There is nothing in production to protect, so this
work moves fast and breaks things: no migration, no back-compat, no deprecation.
Superseded code is deleted, not wrapped.

The work is **phased**. Phase 1 is a walking skeleton that proves the spine —
hub process, `.sindri/` registry, a pod wired with tmux+socket+workspace, and a
user CLI that can inject a "hello" into any named agent. Everything else (the
browser command surface, PR-as-merge-intent, the TUI client, agent2agent
routing) layers onto that spine once it stands.

## Goals / Non-Goals

**Goals:**

- One hub process per repo is the single writer of td/openspec/git/store/`.sindri`.
- The unix socket an agent connects through *is* its identity — no names on the
  wire, no roster visibility, no cross-pod reach.
- The agent runs interactive claude in a named tmux session; the hub delivers all
  inbound messages by `tmux send-keys` ("as if typed"), each stamped with its
  source (`[hub]`/`[user]`/`[reviewer]`).
- Nothing blocks: every socket call returns fast, the agent idles at its prompt,
  and the hub wakes it only by injection.
- The host CLI and TUI are hub clients consuming the same JSON.
- **Phase 1 demonstrable end to end:** `sindri tell <agent> "hello"` appears in
  that agent's tmux pane, routed through the hub, with no other machinery present.
- **The hub is crash-restartable.** It may die at any time and lose nothing
  committed; all state is persisted under `.sindri/`, and pods/tmux outlive it.
- **Human override is primary.** The hub is the human's helper, not the reverse. A
  human SHALL be able to seize direct control of any agent at any time via
  `sindri attach`, and fully owns the hub itself — it runs locally and is trivially
  started/stopped/restarted, so the human is never at the hub's mercy.

**Non-Goals:**

- No migration or back-compat. The `.git/pr` store, the `worker.List` reconciler
  join, the `.sindri-task` block file, and the `claude --bg` daemon are deleted.
- No general agent-addressable relay (`agent send <name>`). Agent2agent stays
  object-mediated for now.
- No remote/multi-host operation. The hub is local, single-host, single repo.
- No auth/TLS — the socket's filesystem location and per-pod mount *are* the
  security boundary.

## Decisions

### D1 — Transport: `net/http` over a unix socket, not gRPC or `net/rpc`

The hub listens with `net.Listen("unix", …)` and serves a stdlib `http.Server`.
JSON bodies; streaming via chunked responses / SSE.

- **Why over gRPC:** this is a local, single-host, same-language (Go both ends)
  tool. gRPC buys cross-language wire efficiency and codegen we don't need, at the
  cost of protobuf, a build step, and `grpcurl` to debug. HTTP-over-unix is
  `curl --unix-socket`-debuggable with zero codegen.
- **Why over `net/rpc`:** no streaming, gob-only, effectively frozen. Disqualified
  by the streamed-stdout and SSE needs.
- **Pod death is free:** when a pod dies the connection drops and the handler's
  `Request.Context()` cancels — no bespoke keepalive/cancellation plumbing.

Endpoints (filled in across phases): `POST /exec` (argv → streamed
stdout/stderr/exit), `GET /commands` (state-filtered surface), `GET /state`
(board JSON), `GET /events` (SSE).

### D2 — Socket is identity; one socket per pod

Each pod mounts exactly one unix socket (the agent's own). The hub derives the
caller's identity from *which listener accepted the connection*, not from any
field the client sends. Mechanically this means **one socket path per agent**
(the hub listens on, or routes per, a per-agent socket), so a compromised or
confused agent simply has no way to name another. The roster, roles, and other
pods are invisible to the agent.

Alternative considered — a single shared socket with a client-supplied agent id —
rejected: it puts identity on the wire where it can be spoofed and forces the hub
to trust the client.

### D3 — Registry + durable state: SQLite at `.sindri/hub.db`, hub-owned

**All** hub state lives in a single SQLite database, `.sindri/hub.db` (gitignored,
separate from td's task DB): the roster (`name`, `role`, `created_at`, workspace,
socket) **and** each agent's live workflow state (current task, merge-intent,
verdict, idle/busy), routing, and an **append-only per-agent activity log** (D12).
Every mutation is a transaction, so multi-fact
changes commit atomically or not at all — this is what makes the crash-restart
guarantee (D11) correct *by construction* rather than hand-rolled. The hub's
in-memory structures are a rebuildable projection of the DB, never the sole source
of truth. This supersedes `add-agent-index`'s `.sindri/agents/*.json` files **and
reverses its "identity-only, never task/status" rule** — under a crash-restartable
hub, task/status are durable too, as rows in `hub.db`. The position-based
reconciler is gone: a restarted hub reloads from `hub.db` plus live pod inspection,
never by inferring from position.

The DB layer is single-owner (only the hub touches it), so per D10 it lives at
`internal/hub/store/`, **not** under `internal/adapter/` — SQLite is a linked
library, not a shelled-out external tool.

`worker` vs `reviewer` is just the `role` field. Mounts are identical; behaviour
divergence is entirely hub-side (which commands `/commands` offers, what the hub
does with a merge-intent). Because the agent binary is a thin browser with no
built-in subcommands and cannot see its own role, there is nothing to distinguish
a worker binary from a reviewer one: **`cmd/sindri-review` is deleted and
`cmd/sindri-worker` becomes the single, role-agnostic agent client.** The hub
alone differentiates the two roles.

### D4 — Agent runtime: interactive claude in a named tmux session

The pod entrypoint starts claude **interactive** inside `tmux new-session -s
<agent>` (replacing `claude --bg … -p`). The hub injects input with
`podman exec <pod> tmux send-keys -t <agent> -- "<msg>" Enter`. tmux gives a
persistent, attachable, `capture-pane`-able terminal, and `send-keys` is what
makes injection behave "as if the user typed."

- **Provenance:** the hub prepends a source tag to every injected line
  (`[hub] …`, `[user] …`, `[reviewer] …`). One merged input stream, still legible.
- **Observability bonus:** `tmux capture-pane`/`pipe-pane` can later feed the
  TUI's live agent view and the existing replay machinery.

### D5 — No blocking; inject-at-idle is the delivery model

Every hub call returns immediately. `submit` records merge-intent and returns
("PR registered. You'll be informed. Please wait."). The agent then does nothing —
an idle claude prompt is its resting state. The hub wakes it by injecting the
next task / the verdict / steering when ready. This is the single decision that
deletes held sockets, keepalives, cancel-on-block, and the `.sindri-task` file.
The agent's standing instruction: *"Act, report, then wait — the system will tell
you what's next, and that may take a long time."*

### D6 — Hub as switchboard; agent2agent is object-addressed

One delivery primitive (D4 injection), three senders: the hub, the user
(`sindri tell <name> "…"`), and another agent. An agent never names a peer — it
acts on a shared **object** (a PR/task) and the hub routes the consequence:
`pr reject pr-abc` → hub resolves branch → owning agent → pod → injection. The
hub is the only holder of the name→pod / object→pod routing table (it owns
`.sindri/workers/`).

### D7 — PR = merge-intent

A "local PR" is a flag: *the agent would like its branch merged*. The hub, with
full host access, lints/reviews/merges. The `.git/pr` store collapses into hub
state — a branch, a wants-merge flag, and a verdict. (Layered in a later phase;
Phase 1 ignores PRs entirely.)

### D8 — Hub lifecycle: ephemeral for CLI, persistent for agents

The socket path is the singleton: bind succeeds ⇒ you own the hub; `EADDRINUSE` ⇒
attach to the running one. A user CLI command with no hub up spawns an
**ephemeral** hub, serves, and exits. A hub with live agents **persists** (agents
need it). If an agent's hub is down, the agent's call fails loudly and tells the
user to start `sindri hub`. The TUI **refuses to start** without a running hub.

### D10 — Package layout: adapter discipline + ownership-based placement

Two structural rules govern where code lives, enforced as a dependency DAG (no
cycles):

1. **One adapter package per external tool; logic never shells out directly.**
   Every external binary/service is wrapped in its own package under
   `internal/adapter/<tool>`, and no logic package calls the tool itself. This
   change introduces the adapters the hub needs:

   ```
   internal/adapter/
     td/        (exists)
     openspec/  (exists as adapter/spec — renamed for consistency)
     git/       NEW — today git is exec'd inline in submit.go, store, worker
     pod/       NEW — podman run/exec/ps/rm, today inline in lifecycle.go
     tmux/      NEW — new-session, send-keys (provenance inject), capture-pane
   ```

   The hub is the only caller of these adapters; agents reach them only via the
   hub. `net/http`+unix-socket is stdlib, not an external tool, so it needs no
   adapter.

2. **Placement by ownership.** A package used by exactly one owner lives as a
   **subdirectory of that owner** (e.g. the hub's command registry →
   `internal/hub/registry/`). A package with more than one owner — adapters,
   shared tools, helpers, generics — lives at **`internal/<package>`** (e.g.
   `internal/adapter/*`, and the thin socket client shared by `sindri-worker`,
   the host CLI, and the TUI → `internal/client`).

   ```
   cmd/* ──▶ internal/hub ──▶ internal/adapter/{git,pod,tmux,td,openspec}
     │            ▲                        (leaves: no internal deps)
     └─▶ internal/client ──▶ (socket only)
   internal/tui ─▶ internal/client
   ```

   Alternative considered — keep adapters as subpackages of whoever first needs
   them — rejected: it hides shared ownership and invites import cycles. Shared
   ownership is the signal to promote a package to `internal/<name>`.

### D15 — Tasks are a cached read model; read fast, write through the tool

Like the roster, abstract tasks live in `hub.db` as a fast local **read model**,
synced from their source of truth (td or GitHub) at startup, periodically, and on
user refresh. UIs read the cache — no shelling per query (faster), and one place to
read, consistent with `GET /state`. Writes go to the source of truth through its
tool, and the hub write-throughs the cache. CQRS-lite: read path = cache, write path
= tool.

For the td backend specifically, the read path MAY open **td's own SQLite directly**
(fast), while **every write goes through the `td` tool** (correctness — never write
td's schema directly). Both strategies are encapsulated in `internal/adapter/td`, so
logic still sees one adapter (D10 holds); reads-bypass-tool is a deliberate,
encapsulated exception, not a leak.

- **Staleness is bounded at the decision points, not by polling.** Lists/board are
  served from cache (fine to be slightly stale for browsing). The cache is refreshed
  from the source of truth exactly where staleness would mislead or cause a wrong
  decision: **all tasks at startup**, **the task right before it is assigned** (never
  hand out a task that has already changed), and **the task right before its detail
  is shown**. Periodic background sync is optional on top. The hub's own writes are
  always write-through, so only external mutations can stale the cache.
- **Schema coupling:** direct-reading td's DB couples us to td's schema; contained
  in the adapter, revisited if td changes.

### D14 — The roster is declarative; orphans are unaccounted runtime

The logic flips from *observed* to *declared*. Agents are not discovered by
enumerating containers/worktrees and inferring a roster (the old reconciler) — they
are **declared** as rows in `hub.db`, and reality is checked *against* the
declaration. The residue — a pod (or worktree) running with no roster row — is an
**orphan**: runtime nothing accounts for. The hub surfaces orphans as a warning and,
rather than auto-killing them, **proposes a shell command** the user can run to
remove them (human decides — matches human-primacy and move-fast; no prune
subsystem). This supersedes `add-agent-index`'s `worker.Orphans` / `RemoveOrphan` /
`worker prune` flow.

|             | in roster            | not in roster                |
|-------------|----------------------|------------------------------|
| pod running | healthy agent        | ORPHAN → warn + propose kill |
| no pod      | stopped / launchable | (nothing — does not exist)   |

### D13 — Identity precedes runtime; launch binds a body and rehydrates

An agent is a row in `hub.db` **first**; the pod is disposable runtime that
*assumes* that identity by mounting the agent's socket (D2). An agent can exist
with **no pod** — pre-declared, stopped, or crashed. `sindri new <name>` registers
an identity; `sindri launch <name>` tells the hub to spin a pod for an existing
identity. On launch (or relaunch after a crash), the hub MAY **rehydrate** the
agent by injecting a briefing built from the tail of its activity log (the last N
events), so a fresh Claude session knows what it was doing. The body is mortal; the
identity and its history are not — a new body picks up where the last left off.

This is the agent-side recovery story for D11: a restarted hub reads the roster,
relaunches pods for agents that should be running, and rehydrates each from its
log.

- **Fork — what to inject:** raw last-N events (mechanical, MVP) vs. a hub-composed
  digest (cleaner, more work). Start raw, refine to a digest later.
- **Canonical resume source is the hub log**, keeping the pod stateless and the hub
  authoritative. Claude's own mounted `.claude` session MAY persist the freeform
  chat as a bonus but is NOT relied upon for resume.

### D12 — Durable per-agent activity log

The hub records an append-only activity log per agent in `hub.db` (an `events`
table: agent, timestamp, type, payload). It captures everything **hub-mediated** —
the agent's socket calls and their results, every injected message with its
provenance tag, merge-intent registrations and verdicts, and status transitions —
giving a complete, replayable timeline of what each agent did. This is the
*structured* hub↔agent stream, deliberately **not** the freeform Claude
reasoning/chat in the pane (that stays observable live via `attach` and
`capture-pane`). The TUI renders this log as a per-worker timeline so a human can
see at a glance what a worker has been doing. Append-only means the log is also the
spine of any future audit/replay.

### D11 — The hub is crash-restartable; runtime outlives it

The hub MAY crash or be restarted at any time and MUST lose nothing committed.
Because every state change is write-through-durable (D3), a fresh hub rebuilds
full operating state from `.sindri/` plus `podman ps` / tmux inspection. Crucially
the agent pods and their tmux sessions run in podman, **independent of the hub** —
a hub crash does not touch them. Agents sit idle in their panes across the blink;
the restarted hub re-resolves pods and resumes injecting, and agents need never
know it restarted. The only thing a crash drops is any in-flight `/exec` stream,
which is simply re-issued. In-memory state is never authoritative; it is a cache
of the durable store.

### D9 — Phasing

1. **Walking skeleton (this milestone):** hub process + unix socket; `.sindri/`
   scaffold and `workers/<name>.json` registry; pod launched with tmux + the
   agent's socket + its workspace; `sindri tell <name> "hello"` routed host→hub→
   `send-keys` into the named pane. Provenance stamp included from the start.
   *Demo:* a human types `sindri tell brokkr "hello"` and sees `[user] hello` in
   brokkr's tmux pane.
2. **Browser client + command registry:** `GET /commands` + `POST /exec`; thin
   `sindri-worker` with no built-in subcommands; state-filtered affordances.
3. **Workflow + PR-as-merge-intent:** act→report→idle loop, `submit` records
   intent, hub lints/reviews/merges; delete `.git/pr` store and reconciler.
4. **TUI/CLI as clients:** `GET /state` + `GET /events`; rip the TUI off direct
   td/store; TUI requires a hub.
5. **Cleanup/retire:** delete `role-driven-launch` and `hot-swap-agent-tooling`
   change dirs; fold surviving "shed baked instructions" intent in; reframe the
   `add-agent-index` specs.

## Risks / Trade-offs

- **tmux `send-keys` quoting / injection.** Arbitrary message text (and the source
  tag) must be escaped so it can't break out of the keystroke stream or inject
  control sequences. → Use `send-keys -l --` (literal) for the body and a separate
  `Enter`; never interpolate untrusted text into a shell-parsed `-c`.
- **Interactive claude vs. injected input timing.** Keys land in claude's input
  queue; if claude is mid-tool-call they're processed after it returns. → The
  no-blocking model (D5) keeps the agent idle between actions, so injected input is
  picked up promptly; document that mid-action injection is queued, not interrupt.
- **Socket-per-agent listener fan-out.** One listener per pod is simple but the hub
  must manage N listeners and map each to an identity. → Acceptable at this scale
  (handful of agents); revisit only if agent counts explode.
- **Ephemeral-vs-persistent hub races.** Two CLIs racing to spawn a hub. → Bind on
  the socket path is the lock; the loser gets `EADDRINUSE` and attaches.
- **Single writer = restart, not catastrophe.** A hub crash strands agents only
  until it restarts. → All state is write-through-durable (D3/D11) and pods/tmux
  outlive the hub, so a restarted hub recovers full state and resumes; nothing
  committed is lost. The cost is on the write path: every mutation must be
  persisted before it is acknowledged.
- **Pane as the only inbound path.** If tmux/`send-keys` misbehaves, the agent is
  unreachable. → Phase 1's whole job is to prove this path solid before anything
  depends on it.

## Open Questions

- Does the hub listen on one socket per agent (clean identity, more listeners) or
  multiplex per-pod sockets onto one accept loop? (Leaning per-agent socket.)
- Where do agent **instructions** come from once the binary is thin — hub-served
  on first `/exec`, or still a file in the image? (Folds in from
  `hot-swap-agent-tooling`; decide in Phase 2.)
- ~~Storage format for durable state~~ — **Decided: a single SQLite DB at
  `.sindri/hub.db`** (D3). ACID multi-fact commits make the crash-restart guarantee
  correct by construction; readability is served by `/state`, not the on-disk
  format. Open sub-question: exact schema/migrations approach.
