# Sindri

A sandboxed AI-agent orchestrator. Agents run inside containers, pick up tasks,
write code, and open PRs; a human approves the merge — the one hard gate. A
single per-repo **hub** owns all state and mediates everything.

---

## Concept

Everything goes through one process — the **hub** (`sindri hub`), a per-repo
service bound to a unix socket. The hub is the only thing that touches the task
store, git, and the agent registry. Agents, the CLI, and the TUI are all thin
clients of it.

```
┌──────────────────────────────────────────────────────────────┐
│ Host                                                           │
│                                                                │
│   sindri CLI ─┐                          ┌─ sindri tui         │
│   (you)       │                          │  (live board)       │
│               ▼                          ▼                      │
│            ┌──────────────────────────────────┐                │
│            │  sindri hub   (single writer)     │                │
│            │  .sindri/hub.db  (SQLite)         │                │
│            │  td · git · openspec · podman     │                │
│            └───────┬───────────────┬──────────┘                │
│         per-agent  │ unix socket   │ tmux send-keys             │
│            ┌───────▼───────┐   ┌───▼───────────┐                │
│            │ pod: brokkr   │   │ pod: reviewer │   …            │
│            │  Claude+tmux  │   │  Claude+tmux  │                │
│            │  /workspace   │   │  /workspace   │                │
│            └───────────────┘   └───────────────┘                │
└──────────────────────────────────────────────────────────────┘
```

The agent inside a pod runs a thin **browser** (`sindri-worker`) with *no
built-in commands*: it asks the hub what it can do and forwards verbs for the
hub to execute. The socket an agent connects through **is its identity** — no
names on the wire, no visibility of other agents.

---

## The loop

```
1.  sindri-worker next            ← claim the top task, branch in /workspace
2.  edit /workspace               ← the agent writes code (the hub commits)
3.  sindri-worker submit "…"      ← register a merge-intent; returns at once
4.  …idle…                        ← the agent waits; no polling, no blocking
5.  reviewer approves / rejects   ← rejection feedback is typed back to the agent
6.  sindri merge <pr>             ← human-only: the one hard gate
7.  [hub] verdict typed in        ← "merged — run sindri-worker next"  → goto 1
```

`submit` never blocks. The agent reports and goes idle; the hub wakes it by
typing the next instruction into its tmux session. A long wait is expected.

---

## Key ideas

- **Single writer.** The hub is the only writer of `td`, git, and `.sindri/`, so
  there are no races. Every UI reads the same `GET /state` and live-updates over
  `GET /events`.
- **Identity is the socket.** Each pod mounts one socket (`.sindri/sockets/
  <name>.sock`); the hub knows who is calling by which socket accepted the
  connection.
- **Server-driven commands.** `sindri-worker` with no args lists what's possible
  *right now* — filtered by role and state. A command you can't run is invisible.
- **Provenance.** Every message the hub types into an agent is tagged `[hub]`,
  `[user]`, or `[reviewer]`.
- **Identity precedes runtime.** An agent is a row in `hub.db`; the pod is a
  disposable body that assumes it. Relaunch resumes from the activity log.
- **PR = merge-intent.** "Submit" just flags a branch for merge; the hub lints,
  the reviewer judges, the human merges.
- **Crash-restartable.** All state is durable in `.sindri/hub.db`; pods/tmux
  outlive a hub restart.

---

## Quick start

```bash
make install                 # builds sindri + sindri-worker, builds the image

sindri hub &                 # start the per-repo hub (agents need it running)

sindri new brokkr            # register a worker identity (no pod yet)
sindri new rune --role reviewer
sindri launch brokkr         # spin its pod (runs interactive Claude)
sindri launch rune

sindri agents                # see the board (or: sindri tui)
sindri tell brokkr "focus on the parser first"   # steer any agent live
sindri attach brokkr         # dial into its live terminal

sindri prs                   # pending merge-intents
sindri merge pr-td-abc123    # approve → merge (human gate)
```

Use `sindri launch <name> --shell` to run a bare shell instead of Claude (for
demos/debugging). `make demo` / `make loop` drive a throwaway repo end to end
(need podman).

---

## Commands

| Command | What |
|---|---|
| `sindri hub` | run the per-repo hub service (foreground) |
| `sindri new <name> [--role]` | register an agent identity |
| `sindri launch <name> [--shell]` | spin a pod that assumes the identity |
| `sindri tell <name> "msg"` | type a message into an agent's session (`[user]`) |
| `sindri attach <name>` | share an agent's live tmux terminal |
| `sindri agents` | list agents + workflow state + orphan warnings |
| `sindri prs` | list merge-intents |
| `sindri merge <pr>` | merge an approved PR (human-only) |
| `sindri tui` | live board (a hub client) |

Inside a pod the agent uses the browser `sindri-worker` (`next`, `submit`,
`approve`/`reject`, `status`, `log`, `prs`) — the surface the hub offers it.

---

## State

| What | Where |
|---|---|
| Roster, workflow state, PRs, activity log | `.sindri/hub.db` (SQLite, gitignored) |
| Per-agent socket | `.sindri/sockets/<name>.sock` |
| Agent Claude home | `.sindri/claude/<name>/` |
| Code / commits | `.worktrees/<name>` (host) |
| Tasks (source of truth) | `td` (cached into `hub.db`) |

Throw a pod away freely; relaunch resumes from the log. Restart the hub freely;
nothing committed is lost.

---

## Repo layout

```
cmd/sindri/         host CLI (hub verbs + tui + lint)
cmd/sindri-worker/  the agent's thin browser (no command tree)
internal/hub/       the hub: service, SQLite store, command registry, routing
internal/client/    thin hub client (CLI + TUI share it)
internal/adapter/   one package per external tool: git, pod (podman), tmux, td, spec
internal/tui/       lean Bubble Tea dashboard (a hub client)
container/          the agent image (Dockerfile) + tmux entrypoint
openspec/           the spec-driven design (changes/hub-architecture)
```

---

## Acknowledgments

The Sindri TUI began as a fork of [sidecar](https://github.com/marcus/sidecar)
by Marcus; the current dashboard is a lean rewrite against the hub.
