## Context

Sindri runs one hub daemon per repo, bound to `<repo>/.sindri/hub.sock`, with all
state in `<repo>/.sindri/hub.db`. `h.root` (a single repo root) threads through the
Hub struct and ~7 hub files; agents are identified by their per-repo socket (Linux)
or a per-repo token (macOS). This sprawls across repos and offers no cross-repo view.

Two facts make a global hub tractable rather than a rewrite:
- The **pod layer is already repo-scoped**: container names include `repoTag(root)`
  (an 8-hex digest of the abs path), so containers never collide across repos.
- **td/git/worktree adapters already take a `root`/dir argument** — they're per-call,
  not bound to a single hub root.

And today's macOS work already built the transport a global hub needs: one TCP
socket with identity carried as a token on the wire, rather than by socket path.

sindri is a prototype — no users, no data to preserve — so this is a clean break.

## Goals / Non-Goals

**Goals:**
- One global hub daemon serving every repo, with one control socket and lifecycle.
- Central state under `~/.local/state/sindri` (`$XDG_STATE_HOME`); repos hold no hub state.
- One project-keyed store; agent identity unique per `(project, name)`.
- Clients carry repo context on every request; the hub scopes/aggregates by project.
- A unified TUI: global Agents/PRs tabs, per-repo Tasks tab, a repo switcher.

**Non-Goals:**
- Migration from the per-repo layout or any backwards compatibility (nothing to migrate).
- Moving `.worktrees/` or `.todos/` out of repos — they stay (git-owned / td-owned).
- Multi-machine / networked hubs — this is one hub per machine, loopback only.

## Decisions

### D1 — Single global daemon at a machine-level socket
One hub bound to a control socket under the runtime dir (`$XDG_RUNTIME_DIR/sindri/hub.sock`
on Linux; state dir on macOS, which lacks `XDG_RUNTIME_DIR` — and the control socket
is host-only, so the podman VM boundary doesn't constrain it). The pid/version file
and `hub list`/`stop`/`start` semantics carry over, now describing the one hub.
*Alternative rejected:* keep per-repo hubs but add a cross-repo aggregator — leaves
the sprawl and a second moving part.

### D2 — One project-keyed store (Strategy B), not per-repo DBs
A single `hub.db` under the state dir; every currently-per-repo table gains a
`project` column (value = `repoTag`), and `agents` becomes `(project, name)` PK.
Reads/writes scope by project; the global board is a single query. *Alternative
(Strategy A):* the hub holds `map[repo]*Store` over per-repo DBs — zero store
rewrite, but the unified board must aggregate in Go and identity stays awkward.
With no migration cost, B's one-time rewrite of the ~35 store methods buys the
cleaner end state and the natural global board; chosen.

### D3 — Repo context travels on the wire
The client resolves the repo root (from cwd, as today) and sends it on every
repo-scoped request — an `X-Sindri-Project` header carrying the repo root (hashed to
`repoTag` server-side). Agent requests derive their project from their identity
(socket/token), never from a client-supplied value. *Alternative rejected:* one hub
process per repo behind a router — that's just per-repo hubs again.

### D4 — Identity becomes (project, agent)
The macOS token derivation folds the project in: `HMAC(hub-secret, project + "\x00" + name)`;
the hub builds a `token → (project, agent)` map from the registry. Linux keeps
per-agent unix sockets, now rooted under the central state dir and resolved to
`(project, agent)`. The hub-global secret and TCP port stay in `meta`.

### D5 — Central layout
- State: `$XDG_STATE_HOME/sindri/` (`~/.local/state/sindri`; `SINDRI_HOME` overrides) —
  `hub.db`, `hub.log`, per-project subdirs (`<repoTag>/`) holding agent Claude-homes
  and per-agent sockets.
- Runtime: `$XDG_RUNTIME_DIR/sindri/` (Linux) for `hub.sock` + `hub.pid`; state dir on macOS.
- Cache: unchanged (`os.UserCacheDir()` for the image build).
- Go has no state/runtime helper, so a small `paths` package resolves these with XDG
  env + fallbacks. macOS `$HOME`-rooted state means agent homes still mount into the VM.

### D6 — Projects registry
A `projects(repoTag, path, first_seen)` table (or `meta` rows). Registered on first
request carrying a repo. Source of truth for `repoTag → path` (worktree creation,
UI labels) and for the switcher's list.

### D7 — Board split
`BoardState` becomes global `Agents`/`PRs` (each tagged with `project`/repo) plus
`Tasks` for the requested project. `/state` takes the selected repo; `/events`
streams global agent/PR changes. Worktrees stay at `<repo>/.worktrees/<agent>`; the
hub resolves the repo path from the registry to place them.

### D8 — `sindri hub status` replaces `hub list`
There is one hub, so `sindri hub list` becomes `sindri hub status`: the running hub
(pid, version, uptime) plus the projects it currently serves. `start` / `--bg` /
`stop` are unchanged. *Alternative rejected:* keep `hub list` — misleading now that
there's nothing to enumerate but one hub.

### D9 — Repo switcher as a picker overlay, with per-project color identity
The switcher is a picker overlay (scales past a couple of repos, discoverable) rather
than a cycle key. Each project also gets a **deterministic color scheme** so you can
tell at a glance which repo you're looking at. A scheme is a *(primary, accent)* pair
chosen from a fixed palette by hashing the project key (`repoTag`) — using two
independently-varied colors makes the space `primary × accent`, big enough that the
handful of repos in use rarely collide. The current project's scheme tints the board
chrome (active-repo indicator / header) and per-row repo tags, so agents/PRs from
different repos are visually separable. Schemes live in the UI-neutral render layer,
keyed by `repoTag` (deterministic, no persistence). *Alternative rejected:* random or
sequential color assignment — not stable across sessions, so a repo's color would drift.

## Risks / Trade-offs

- **Blast radius of the route rewrite** (~32 routes, ~36 client methods, ~35 store
  methods) → do it as a mechanical, typed pass: add `project` to store signatures
  first (compiler finds every call site), then the header plumbing, then the board.
- **One crash affects all repos' orchestration** (vs today's isolation) → agents/pods
  survive hub restart (unchanged invariant), and the pid/version restart tooling
  already exists; acceptable at prototype scale.
- **Global single writer becomes a machine-wide bottleneck** → fine for a handful of
  repos and a single SQLite writer; revisit only if it ever matters.
- **A repo outside `$HOME` on macOS** would have its in-repo worktree unshared with the
  VM → document that repos live under `$HOME` (or add a podman machine mount) — same
  constraint as today.

## Migration Plan

None. Prototype with no users or data: delete the per-repo `.sindri/` code paths and
ship the central layout. Any existing local `.sindri/` dirs can simply be removed by
the developer.

## Open Questions

Resolved during proposal review:
- **Repo context transport** → `X-Sindri-Project` header (D3), smallest diff.
- **`hub list`** → `sindri hub status` (D8).
- **Repo switcher** → picker overlay, plus deterministic per-project color schemes (D9).
- **Store** → one project-keyed DB, Strategy B (D2).

None outstanding.
