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

- [x] 3.1 Hub owns merge-intent state (`prs` table: branch + status + verdict);
      `submit` records it and returns immediately ("registered, please wait")
- [x] 3.2 Agent loop: act → report → idle; hub injects next task / verdict (no
      blocking, no `.sindri-task` file, no polling) — `submit` returns, worker idles,
      hub wakes it on merge/reject
- [x] 3.3 Hub runs lint/review/merge host-side: `submit` runs the lint gate via
      `sindri lint all` in the worktree (tolerant — Go modules only; non-Go skips);
      reviewer approve/reject + human `merge` via `adapter/git`
- [x] 3.4 Object-mediated agent2agent: `reject` resolves branch→owner→pod →
      injects `[reviewer]` feedback; `merge` routes `[hub]` back (verified live)
- [x] 3.5 **Deleted** `internal/ghlocal/store` and the `worker.List` reconciler
      (done in Phase 4's sweep): removed `ghlocal`, `worker`, `board`, `action`, the
      old `tui`, `cmd/timing`, and trimmed `render`(gone)/`issue`/`td`/`spec` to what
      the hub uses
- [x] 3.6 Live workflow state persisted write-through in `hub.db` (`agent_state`,
      `prs`, `tasks`); a restarted hub re-serves agent sockets + reads state from the
      DB (in-memory is a projection)
- [x] 3.7 Log workflow events (claim/submit/approve/reject/merged) to `events` via
      `Log(...)` — verified in the activity log
- [x] 3.8 Rehydrate on (re)launch (D13): `go h.rehydrate` injects a briefing from
      the tail of the `events` log via `injectWhenReady`
- [x] 3.9 Task read model (D15): cache in `hub.db` (`tasks`), synced from td;
      refresh-before-assignment in `next`; `adapter/td` writes via the `td` tool;
      reads go **directly** from td's SQLite (`adapter/td/sqlite.go`), bypassing the
      CLI on the hot path. (Also fixed a latent store ordering bug — td priorities
      are `P1`..`P4`, not words.)

## 4. Phase 4 — TUI/CLI as clients

- [x] 4.1 `GET /events` SSE stream of state changes (hub pub/sub `bus`; `notify()`
      after every mutation; client `Watch` consumes it)
- [x] 4.2 **Replaced** the legacy TUI with a lean hub client (`internal/tui`): one
      Bubble Tea model that reads `/state` and live-updates over `/events` via
      `internal/client`. Deleted the old TUI/board/action stack
- [x] 4.3 TUI refuses to start without a running hub (`tui.Run` guard + test)
- [x] 4.4 CLI (`sindri agents`/`prs`) + TUI render the same fields from the same
      `BoardState` payload
- [x] 4.5 Per-worker activity timeline in the TUI from the hub's `events` log
      (`GET /log`, Enter on an agent); not the freeform pane chat
- [x] 4.5b Refresh action: TUI `r` (and `POST /refresh`) re-syncs tasks from the
      source of truth into `hub.db`
- [x] 4.6 Orphan detection (D14): `State` diffs `podman ps` (by project label)
      against the roster; `/state.Orphans` lists pods with no roster entry; CLI +
      TUI show an "orphan" warning + a proposed `podman rm -f …`; never auto-kills

## 5. Phase 5 — retire superseded changes + cleanup

- [x] 5.1 Deleted `openspec/changes/role-driven-launch` and
      `hot-swap-agent-tooling` (superseded by this change)
- [x] 5.2 Closed `add-agent-index`: its `.sindri/agents` roster + reconciler were
      deleted in Phase 4 and its intent is reframed in this change's specs; the
      change dir is removed (also removed the now-moot old-TUI changes
      `auto-parent-on-create`, `polish-help-bar-keys-grouping`)
- [x] 5.3 Deleted baked agent tooling — `container/skills/`, `CLAUDE.md`,
      `CLAUDE.reviewer.md`, `wait-for-task.sh` + their Dockerfile `COPY`s; the agent
      is a thin browser whose instructions come from the hub
- [x] 5.4 `cmd/sindri-review` already gone (Phase 2); image bundles only the single
      `sindri-worker` browser + tmux + the `sindri-agent` entrypoint

## 6. Validation

- [x] 6.1 `openspec validate hub-architecture --strict` passes
- [x] 6.2 `go test ./...` green; `sindri lint all` green; no import cycles
- [x] 6.3 End-to-end verified two ways: `make loop` (deterministic, --shell) and
      `make fullloop` — TWO real Claude agents autonomously: worker `next`→writes
      file→`submit`; the reviewer is notified, reasons about the change, and
      `approve`s; the human merges; `[hub]` verdict routes back to the worker; td
      task closed. (`reject`→`[reviewer]` routing in `cmdReject`.)

## Follow-ups (outside this change)

- [x] Wire interactive Claude as the agent's tmux command (the hub prepares the
  agent's Claude home + credentials + a role-aware system prompt; the entrypoint
  runs `claude --dangerously-skip-permissions`; `--shell` is the demo/debug
  escape hatch). Verified live: a real Claude worker ran `next` → wrote a file →
  `submit` autonomously from the hub kickoff.
- README.md / AGENTS.md still describe the pre-hub architecture (gh-local backend,
  `sindri work`, baked skills) — they need a rewrite for the hub model.
- (Both former deferrals are now done: lint gate at `submit` (3.3) and direct
  td-SQLite reads (3.9).)
