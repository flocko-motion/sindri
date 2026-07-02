## 1. Central paths

- [x] 1.1 Add an `internal/paths` package resolving the state dir (`$XDG_STATE_HOME/sindri`, fallback `~/.local/state/sindri`), runtime dir (`$XDG_RUNTIME_DIR/sindri` on Linux, state dir on macOS), and honoring `SINDRI_HOME`; keep the image cache on `os.UserCacheDir()`.
- [x] 1.2 Move the control socket + pid/version file to the runtime dir; update `SocketPath`/`IsRunning`/`HubPID`/`WritePID` to the machine-level location (no `root` arg).
- [x] 1.3 Delete the in-repo `.sindri/` machinery: `ensureSindriIgnore`/`.sindri/.gitignore`, and the `.sindri/` entry from `ensureGitignore` (keep `.worktrees/`).

## 2. Project-keyed store

- [x] 2.1 Add a `project` column to every per-repo table (agents, agent_state, prs, events, tasks-cache, task_priority, reviews, pr_lint, pr_events, task_approval); make `agents` PK `(project, name)`. Keep `meta` hub-global.
- [x] 2.2 Scope every query/insert by project — via a `ProjectStore` handle (`store.For(project)`) rather than a per-method arg (design D2 refinement); global board reads (`AllAgents`/`AllPRs`) stay on `*Store`.
- [x] 2.3 Add a `projects(repoTag, path, first_seen)` table with `RegisterProject`, `ProjectPath(repoTag)`, and `Projects()`.
- [x] 2.4 Point `store.Open` at the single central `hub.db`.

## 3. Hub core (single global, project-aware)

- [x] 3.1 Drop `h.root`; make the Hub hold central paths + the one store. Per-agent maps (`agentLn`, `lifecycle`, `launchBuf`) key on `(project, name)`.
- [x] 3.2 Rework `New`/`Serve`/`Close` for one global instance (no per-repo root); register a project on first request.
- [x] 3.3 Rehome agent Claude-homes and per-agent sockets under `<state>/<repoTag>/`; resolve worktree paths to `<repo>/.worktrees/<agent>` via the registry.
- [x] 3.4 Fold the project into macOS token derivation (`HMAC(secret, project+"\x00"+name)`) and resolve `token → (project, agent)`; scope Linux socket identity to `(project, agent)`.

## 4. Request routing (repo context on the wire)

- [x] 4.1 Client: resolve repo root (cwd) and send it as `X-Sindri-Project` on repo-scoped calls; dial the one global socket. Thread through the ~36 client methods.
- [x] 4.2 Hub server: middleware/helper that resolves the project from the header (host CLI/TUI) or from agent identity (agent socket/token), and passes it to handlers across the ~32 routes.
- [x] 4.3 Reject/omit project where it doesn't apply (hub-global endpoints) and error clearly when a repo-scoped call arrives without context.

## 5. Board + TUI

- [x] 5.1 Split `BoardState`: global `Agents`/`PRs` tagged with `project`/repo; `Tasks` for the requested project. Update `/state` (takes selected repo) and `/events` (global agent/PR changes).
- [x] 5.2 TUI Agents and PRs tabs: aggregate across projects, add a repo column; live counts stay hub-provided.
- [x] 5.3 TUI Tasks tab: scope to the selected repo.
- [x] 5.4 Add a repo switcher as a picker overlay backed by `Projects()`; selecting a repo rescopes the per-repo view only.
- [x] 5.5 Add a deterministic per-project color scheme in the render layer: a fixed palette, a project's scheme = a `(primary, accent)` pair chosen by `hash(repoTag)`; tint the board chrome with the selected repo's scheme and color per-row repo tags by their project.

## 6. CLI lifecycle

- [x] 6.1 `sindri hub start/--bg/stop`: operate on the one global hub; replace `hub list` with `sindri hub status` (the running hub's pid/version/uptime + the projects it serves).
- [x] 6.2 `coauthor`/`tui` bootstrap: keep auto-start of the global hub; ensure they register/select the cwd's repo.
- [x] 6.3 Update user-facing strings that say "per-repo hub" / point at `.sindri/`.

## 7. Verification

- [x] 7.1 Store tests: project scoping + `(project, name)` isolation (same name in two projects).
- [x] 7.2 Token test: `(project, agent)` derivation + resolution; cross-project token rejected.
- [x] 7.3 Live: two repos under one hub — agents in both, unified Agents/PRs board, per-repo Tasks via the switcher; hub restart resumes both projects.
- [x] 7.4 `make verify` green; `openspec validate global-hub` passes.
