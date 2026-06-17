# Hub architecture

## Why

Today every actor — the agent CLI inside a pod, the user's CLI, and the TUI —
writes td, git, and the PR store directly. Worker isolation is by *instruction
only*: `container/CLAUDE.md` tells the agent "do NOT use `td` directly" while the
td database sits mounted read-write at `/project/.todos`. Nothing enforces it,
and several writers race on the same SQLite DB, git worktrees, and the JSON PR
store.

Invert it. A single per-repo service — **the hub** — becomes the only writer and
the only gatekeeper. Everything else (agents, user CLI, TUI) becomes a thin
client over a unix socket. Isolation stops being a polite request and becomes a
property of the mounts; concurrency races become structurally impossible because
there is exactly one writer.

## What Changes

- **The hub.** `sindri hub` runs a per-repo service bound to a unix socket. It is
  the only process that touches td and openspec and the only writer of git, the
  store, and `.sindri/`. The user CLI spawns an **ephemeral** hub when none is
  running (serve one request, exit); a hub with live agents **persists**.
- **Socket is identity.** Each agent pod mounts exactly one socket. The hub knows
  who is calling by which socket the connection arrived on — agents cannot spoof
  another, enumerate the roster, or see other pods.
- **Thin "browser" client.** The agent binary knows **no subcommands of its own**.
  Listing what it can do is a hub call returning what is *currently possible* for
  that caller, derived from `(role, state)`. In the simplest form the agent runs
  `sindri-worker` with no args and the hub tells it the next action.
- **Roles are hub-side.** Worker and reviewer use **identical mounts**; they
  differ only by a field in the hub's roster (`.sindri/hub.db`), which the agent
  cannot see. The agent sees only its git workspace, named after itself.
- **Agent runs interactive in tmux.** Claude runs inside a named tmux session
  (replacing the `claude --bg … -p` headless daemon). The hub pushes input via
  `podman exec <pod> tmux send-keys` — *as if the user typed it*.
- **No blocking, anywhere.** Every socket call returns fast. `submit` returns
  immediately ("PR registered. You'll be informed. Please wait."); the agent then
  goes **idle** at its prompt. Idle is its resting state, not a held call. The hub
  wakes it only by injecting. **BREAKING:** removes held/long-poll sockets,
  keepalives, cancel-on-pod-death plumbing, and the `.sindri-task` block file.
- **Hub as switchboard.** One delivery primitive (tmux inject), three senders: the
  hub (next task / lifecycle), the user (`sindri tell <name> "…"` — steer *any*
  agent), and another agent. Agent→agent is **object-addressed, never
  peer-addressed**: the reviewer addresses a PR (`pr reject pr-abc -b "…"`) and the
  hub resolves branch → owning agent → pod → injection. Peers stay blind to each
  other.
- **Provenance stamping.** Every injected message carries its source tag —
  `[hub]` / `[user]` / `[reviewer]` — so the single merged input stream stays
  legible and the agent can weight messages by source.
- **PR = merge-intent.** A "local PR" becomes a flag meaning *the agent would like
  its branch merged*. The hub uses its full host access to lint, review, and
  merge. **BREAKING:** the `.git/pr` store model dissolves into hub state (branch
  + wants-merge flag + verdict).
- **Identity precedes runtime; agents are resumable.** An agent is a durable
  roster row in `hub.db`, not a container — it can exist with no pod. `sindri new`
  registers an identity; `sindri launch` spins a pod that assumes it via the
  mounted socket. On (re)launch the hub rehydrates the agent by injecting the tail
  of its activity log, so a fresh body resumes the prior agent's work. The body is
  disposable; the identity and its history persist.
- **Tasks are a cached read model.** Abstract tasks live in `hub.db` as a fast local
  cache, synced from their source of truth (td/GitHub) at startup, periodically, and
  on user refresh; UIs read the cache instead of shelling out per query. For td,
  reads come **directly from td's SQLite** for speed while **writes go only through
  the `td` tool** — both encapsulated in `internal/adapter/td`.
- **Declarative roster; orphans warned, not auto-pruned.** Agents are declared as
  rows in `hub.db`, and reality is checked against the declaration (the inverse of
  the old observe-and-infer reconciler). A pod or worktree with no roster entry is an
  orphan: the hub flags it as a warning and **proposes a shell command** to remove
  it rather than killing it. **BREAKING:** retires `add-agent-index`'s
  `worker.Orphans`/`RemoveOrphan`/`worker prune` machinery.
- **Durable activity log.** The hub records an append-only per-agent log of all
  hub-mediated interaction — the agent's socket calls and results, injected
  messages (with provenance), merge-intents, verdicts, and status transitions — in
  `hub.db`, deliberately excluding the freeform pane chat. The TUI renders it as a
  per-worker timeline so a human can see what each worker has been doing.
- **TUI and CLI are clients.** Both receive the same JSON from the hub. The TUI
  becomes a webapp-style client (`GET /state` snapshot + `GET /events` SSE) and
  **refuses to start without a running hub**. No UI touches td or the store
  directly anymore.
- **Strategy: move fast, break things.** Sindri has no users yet and nothing to
  preserve. This change carries **no migration, no back-compat, and no
  deprecation paths** — superseded code is deleted outright.

## Capabilities

### New Capabilities

- `hub`: the per-repo service — unix socket, socket-as-identity, single-writer
  gatekeeping, ephemeral-vs-persistent lifecycle, the HTTP/JSON protocol
  (`/exec`, `/commands`, `/state`, `/events`), and the state-filtered command
  registry that drives the "browser" affordances.
- `agent-runtime`: how an agent runs — interactive claude in a named tmux session,
  thin browser client with no built-in subcommands, inject-at-idle with no
  blocking, and provenance-stamped inbound messages.
- `hub-routing`: the switchboard — name→pod and object→pod resolution, the user
  `tell` verb to steer any agent, and object-mediated (never peer-addressed)
  agent-to-agent delivery.

### Modified Capabilities

- `01-architecture`: reframe from shared-state to hub-mediated — logic stays
  headless and UI-neutral, the hub is the process that hosts it, and all UIs and
  agents are thin clients; external tools are reached only through adapters owned
  solely by the hub.
- `04-workers`: identical mounts for every agent, role held hub-side, the
  position-inferring reconciler removed (the hub is live and authoritative), and
  PR-as-merge-intent.
- `05-workflow`: the agent loop becomes act → report (fast) → idle-and-wait; the
  hub delivers the next task, the verdict, and steering by injection.
- `03-gh-local`: the local PR store dissolves into hub state; the agent's
  workflow verbs are served by the hub rather than executed in the pod.
- `view-workers`: the workers view is rendered from hub-delivered JSON, with status
  owned by the hub rather than reconciled from container/worktree position.

## Impact

- **New:** `internal/hub` (service, socket listener, command registry, protocol
  handlers, routing) with `internal/hub/store` (SQLite at `.sindri/hub.db`, the
  durable single source of truth), a tmux-based agent entrypoint in `container/`,
  and a thin client used by both `sindri-worker` and the host CLI/TUI.
- **Rewritten:** `internal/worker/lifecycle.go` (single mount topology + socket +
  tmux), `internal/tui` data layer (socket client, not direct td/store),
  `internal/agentcli` (browser client, no command tree). New adapters
  `internal/adapter/{git,pod,tmux}` and `internal/adapter/spec` → `…/openspec`;
  no logic package shells out directly anymore.
- **Collapsed:** `cmd/sindri-review` is deleted; `cmd/sindri-worker` becomes the
  single role-agnostic agent client (role lives hub-side).
- **Deleted:** `internal/ghlocal/store` (PR store), the `worker.List` reconciler
  join, the `.sindri-task` block file, the `claude --bg` daemon path, and the
  per-role baked tooling.
- **Retired changes:** `role-driven-launch` and `hot-swap-agent-tooling` are
  superseded and removed; `add-agent-index`'s reconciler/two-layer spec deltas are
  reframed (its `.sindri/agents/*.json` roster moves into `.sindri/hub.db` as a
  table, role a column; live task/status are durable rows, not memory-only).
- **Dependencies:** unix-socket `net/http` (stdlib), tmux in the container image,
  unchanged podman.
