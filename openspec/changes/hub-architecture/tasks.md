# Tasks

Phased per design D9. **Phase 1 is the walking skeleton and stands alone** — it
must demo end to end (`sindri tell <agent> "hello"` → `[user] hello` in the
agent's tmux pane) before any later phase begins. Strategy is move-fast / no
back-compat: superseded code is deleted, not wrapped.

## 1. Phase 1 — walking skeleton

### 1a. Adapters (leaves, no internal deps)

- [x] 1.1 `internal/adapter/pod` — wrap podman: `Run`, `Exec`, `Ps`, `Rm` (port
      the inline calls out of `worker/lifecycle.go`/`worker.go`)
- [x] 1.2 `internal/adapter/tmux` — `NewSession(name)`, `SendKeys(session, text)`
      using literal `send-keys -l --` + `Enter`, and `Attach`/`CapturePane`
- [x] 1.3 `internal/adapter/git` — minimal surface needed now (worktree add,
      branch, current branch); fuller surface lands in Phase 3

### 1b. Store + scaffold

- [x] 1.4 `internal/hub/store` — SQLite at `.sindri/hub.db` (gitignored); schema +
      roster CRUD (`name`, `role`, `workspace`, `socket`, `created_at`); scaffold
      `.sindri/` on first use
- [x] 1.5 Roster read/list from the DB as the canonical set of agents
- [x] 1.6 `events` table (append-only: agent, ts, type, payload) + a `Log(...)`
      helper, write-through in the same transaction as the action it records

### 1c. Hub service (minimal)

- [x] 1.7 `internal/hub`: bind a per-repo unix socket; `EADDRINUSE` ⇒ "already
      running"; serve a stdlib `http.Server`
- [x] 1.8 Identity from the accepting socket (one socket per agent); map
      connection → roster entry — *done in Phase 2 (see 1.8 under §2): each agent
      has `.sindri/sockets/<name>.sock`; the hub serves one listener per agent and
      derives identity from which socket accepted the connection*
- [x] 1.9 `GET /state` returns the roster as JSON (done). `POST /exec` deferred to
      Phase 2 — Phase 1 delivers `POST /agents`, `/launch`, `/tell` instead, which
      is what the skeleton demo exercises
- [x] 1.10 Routing table from the DB roster: resolve agent name → pod
- [x] 1.11 Inject path: hub → `adapter/pod.Exec` → `adapter/tmux.SendKeys` with a
      **provenance tag** (`[user]`/`[hub]`/`[reviewer]`); append an `events` entry
      for every socket call and every injection (provenance included)

### 1d. Pod runtime

- [x] 1.12 Container entrypoint runs Claude **interactive** inside
      `tmux new-session -s <agent>` (replace the `claude --bg … -p` path)
- [x] 1.13 `worker/lifecycle.go`: single mount topology — workspace (named after
      agent) + the agent's one socket; hub launches the pod via `adapter/pod`

### 1e. Host CLI

- [x] 1.14 `sindri hub` — start the persistent hub for the repo
- [x] 1.15 `sindri new <name> --role` — register an identity (roster row in
      `hub.db`), **no pod** — identity precedes runtime (D13)
- [x] 1.16 `sindri launch <name>` — hub spins a pod that assumes the agent's
      identity via its mounted socket
- [x] 1.17 `sindri tell <name> "msg"` — resolve via hub, inject stamped `[user]`
- [x] 1.18 `sindri attach <name>` — resolve name→pod, `podman exec -it … tmux
      attach -t <name>` (read-only variant available)
- [x] 1.19 Ephemeral-hub-on-demand: a CLI command with no hub running spawns one,
      serves, exits; a hub with live agents persists

### 1f. Phase-1 demo + gate

- [x] 1.20 Manual demo VERIFIED with a real pod: `sindri new brokkr` →
      `sindri launch brokkr` (pod running, tmux session live) → `sindri tell brokkr
      hello …` lands `[user] hello …` as typed input in the pane (capture-pane
      confirmed); activity log shows register→launch→recv. (`attach` shares the same
      `pod.ExecInteractive`+`tmux attach` path; needs an interactive TTY to drive.)
- [x] 1.21 `go test ./...` + `sindri lint all` green; no import cycles (DAG per
      D10)

## 2. Phase 2 — browser client + command registry

- [x] 2.1 `internal/hub/registry` — command set keyed by `(role, state)`
      (`Caller{Agent,Role,HasTask}`; role + Hidden-predicate filtering)
- [x] 2.2 `GET /commands` returns only currently-valid commands for the caller
- [x] 2.3 `POST /exec` runs a real command hub-side and streams stdout/stderr,
      exit code via the `X-Sindri-Exit` trailer
- [x] 2.4 `internal/client` — thin socket client shared by the agent binary + host
      (`DialSocket`, `Commands`, `Exec`)
- [x] 2.5 Collapsed `cmd/sindri-review` into `cmd/sindri-worker`; role-agnostic
      browser with no command tree (`sindri-worker` no-args ⇒ hub `/commands` menu);
      deleted `internal/agentcli`; Makefile/Dockerfile updated
- [ ] 2.6 `internal/adapter/spec` → `internal/adapter/openspec` (rename); hub is
      its only caller — *deferred to Phase 3: spec is still imported by ~10 legacy
      packages; the rename is meaningful only once submit/lint move hub-side and the
      hub becomes its sole caller*
- [x] 2.7 Reviewer surface excludes submit/merge; worker surface excludes
      approve/reject/merge — asserted in `registry_test.go` + `client_test.go`
      (`TestAgentSocketIdentityAndSurface`)
- [x] 1.8 (carried from Phase 1) Identity from the accepting socket — each agent
      has its own `.sindri/sockets/<name>.sock`; a connection on it IS that agent
      (verified live: `status` over brokkr's socket returns brokkr/worker)

## 3. Phase 3 — workflow + PR-as-merge-intent

- [ ] 3.1 Hub owns merge-intent state (branch + wants-merge + verdict); `submit`
      returns immediately ("registered, please wait")
- [ ] 3.2 Agent loop: act → report → idle; hub injects next task / verdict (no
      blocking, no `.sindri-task` file, no polling)
- [ ] 3.3 Hub runs lint/review/merge via `adapter/git` (host-side, full access)
- [ ] 3.4 Object-mediated agent2agent: `pr reject` → hub resolves branch→owner→pod
      → inject `[reviewer]` feedback
- [ ] 3.5 **Delete** `internal/ghlocal/store` and the `worker.List` reconciler join
- [ ] 3.6 Persist all live workflow state (task, merge-intent, verdict) write-through
      in `hub.db`; a restarted hub recovers full state from `hub.db` +
      `podman ps`/tmux and resumes, agents untouched across the blink (crash-restart test)
- [ ] 3.7 Log workflow events (task claimed, merge-intent registered, verdict,
      status transitions) to the `events` table via the Phase-1 `Log(...)` helper
- [ ] 3.8 Rehydrate on (re)launch (D13): hub injects a briefing from the tail of the
      agent's `events` log (raw last-N now; digest later) so a fresh pod resumes;
      hub-log is canonical, Claude's mounted session not relied upon
- [ ] 3.9 Task read model (D15): cache abstract tasks in `hub.db`; lists/board read
      the cache. Targeted refresh from source of truth: all at startup, the task
      before it is assigned, the task before its detail is shown (periodic optional).
      `adapter/td` reads direct from td's SQLite, writes only via the `td` tool
      (write-through to cache)

## 4. Phase 4 — TUI/CLI as clients

- [ ] 4.1 `GET /events` SSE stream of state changes
- [ ] 4.2 Rip `internal/tui` data layer off direct td/store; consume `/state` +
      `/events` via `internal/client`
- [ ] 4.3 TUI refuses to start without a running hub
- [ ] 4.4 CLI + TUI render the same fields from the same hub payload (view-workers)
- [ ] 4.5 Per-worker activity timeline in the TUI, rendered from the hub's `events`
      log (actions, socket messages sent/received, merge-intents, status); served
      via `GET /state`/an events query, not the freeform pane chat
- [ ] 4.5b Refresh action: a user `refresh` (CLI flag / TUI `r`) triggers a re-sync
      of tasks (and roster reality) from the source of truth into `hub.db`
- [ ] 4.6 Orphan detection (D14): hub diffs `podman ps`/worktrees against the roster;
      `/state` flags pods with no roster entry as orphans; TUI shows an "orphaned
      agent" warning + a proposed `podman rm -f …` command; never auto-kills

## 5. Phase 5 — retire superseded changes + cleanup

- [ ] 5.1 Delete `openspec/changes/role-driven-launch` and
      `openspec/changes/hot-swap-agent-tooling` (superseded by this change)
- [ ] 5.2 Reframe/close `add-agent-index`: roster survives as `workers/`, role in
      JSON, live state in the hub; drop its reconciler/two-layer deltas
- [ ] 5.3 Delete baked per-role tooling (skills/CLAUDE worker+reviewer split);
      fold surviving "instructions are served, not baked" intent into hub `/exec`
- [ ] 5.4 Remove `cmd/sindri-review` remnants; image bundles only the single client
      + tmux

## 6. Validation

- [ ] 6.1 `openspec validate hub-architecture --strict` passes
- [ ] 6.2 `go test ./...` green; `sindri lint all` green; no import cycles
- [ ] 6.3 End-to-end: worker picks up an injected task, registers merge-intent,
      goes idle; reviewer rejects; feedback lands `[reviewer]` in the worker's pane;
      human merges on the host
