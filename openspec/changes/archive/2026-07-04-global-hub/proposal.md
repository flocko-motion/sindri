## Why

Today sindri runs one hub daemon **per repo**. That sprawls — stale hubs pile up,
you juggle `hub list`/`hub stop`, and every client dials a different per-repo
socket — and it gives zero cross-repo oversight: you can't see all your agents or
all open PRs in one place. Most people (including us) work a handful of repos at
once and want a single dashboard over them. sindri is a prototype with no users and
no data to preserve, so this is the moment to make it one hub for all repos as a
clean break — no migration, no compatibility layer.

## What Changes

- **BREAKING** One **global hub** daemon serves every repo, replacing the per-repo
  hub. There is one control socket, one process, one `hub start`/`stop`.
- **BREAKING** Hub state moves out of each repo's `.sindri/` to a central
  `~/.local/state/sindri` (`$XDG_STATE_HOME`; the runtime socket + pid under
  `$XDG_RUNTIME_DIR` on Linux, falling back to the state dir on macOS; the image
  build cache stays at `os.UserCacheDir()`; a `SINDRI_HOME` env var overrides the
  root). Removes the in-repo `.sindri/` entirely, including the self-ignoring
  `.sindri/.gitignore`.
- **BREAKING** One central, **project-keyed `hub.db`**: every currently-per-repo
  table (agents, agent_state, prs, events, the td-task cache, reviews, pr_lint,
  pr_events, task_approval) gains a `project` column; the `agents` primary key
  becomes `(project, name)` so two repos can each have an agent named "eitri". The
  token secret and agent TCP port stay hub-global in `meta`.
- Clients send **repo context** (the repo root, resolved from cwd) on every
  request; the hub scopes/aggregates by it. Every hub route and client method
  carries the project.
- Agent identity becomes **`(project, agent)`**: the macOS TCP token derivation
  includes the project (`HMAC(secret, project + name)`) and the hub resolves a
  token to `(project, agent)`. Container names are already repo-scoped via
  `repoTag`, so the pod layer is unchanged.
- A **projects registry** maps `repoTag → repo path`, so the UI shows real repo
  names and the repo switcher has something to list.
- The **TUI becomes a unified cross-repo dashboard**: the Agents and PRs tabs are
  global (each row gains a repo column, aggregated across projects); the Tasks tab
  is per-repo, scoped by a **repo switcher** shown as a picker overlay (td is
  inherently per-repo and a merged backlog is confusing). Each project gets a
  **deterministic color scheme** (a hashed *(primary, accent)* pair) so the current
  repo is identifiable at a glance and cross-repo rows are separable. `BoardState`
  splits into global agents/PRs plus tasks for the selected repo.
- **Unchanged, stays in the repo:** `.todos/` (td's tracked task data) and
  `.worktrees/` (git-owned checkouts — kept local so a repo's full on-disk size is
  measurable; still gitignored). Moving `.sindri/` out also fully resolves the
  earlier credential/token leak concern, since secrets now live centrally outside
  every repo.

## Capabilities

### New Capabilities
- `project-registry`: tracks the set of repos the global hub knows about
  (`repoTag → path`, first-seen), so requests, the board, and the switcher can
  resolve and list projects.

### Modified Capabilities
- `hub`: the hub is a single global daemon over central state (`~/.local/state/sindri`,
  one project-keyed DB) rather than one per repo.
- `hub-routing`: one global control socket; every request carries repo context and
  is scoped/aggregated by project; agent identity becomes `(project, agent)`.
- `agent-runtime`: the per-agent token is derived over `(project, name)` and the hub
  resolves it to `(project, agent)`; agent home/socket paths move under the central
  state dir.
- `view-tui`: a unified cross-repo board — global Agents/PRs tabs with a repo
  column, a per-repo Tasks tab, and a repo switcher.
- `01-architecture`: the "single writer / per-repo hub / in-repo `.sindri`" model
  becomes "single global writer / central state / repo context on the wire".

## Impact

- **Code**: `internal/hub` (Hub struct, `New`, `Serve`, socket/pid, agent server +
  TCP token, store schema + ~35 methods, ~32 HTTP routes, `state.go`), `internal/hub/store`
  (project column + composite keys), `internal/client` (~36 methods carry project;
  global socket), `internal/tui` (tabs, switcher, aggregated board), `cmd/sindri`
  (hub start/list/stop, coauthor/tui bootstrap, path resolution), `internal/adapter/td`
  (already root-parameterized — unchanged), the agent entrypoint (home path).
- **Paths**: new `~/.local/state/sindri` layout; `SINDRI_HOME` override; removal of
  in-repo `.sindri/` and its gitignore machinery.
- **Out of scope**: migration from the old per-repo layout (nothing to migrate) and
  backwards compatibility.
