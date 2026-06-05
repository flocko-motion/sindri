# A durable agent index in `.sindri/`

## Why

There is no explicit record of which agents exist. `worker.List()` reconstructs
everything at read time by joining three sources — podman containers, git
worktrees, and the `td` store — and *infers* role purely by position (the main
repo is forced to be the reviewer; leftover containers become "orphans"). The
only fact an agent writes about itself is `.sindri-task`, a single task ID in
its worktree.

Two problems fall out of this:

- **No source of truth.** A crashed worker, an idle one, and one whose
  workspace vanished all collapse into the same blurry "exited" — the join has
  attendance (what containers/worktrees happen to exist) but no roll call (what
  agents are *supposed* to exist).
- **Identity dies with the workspace.** Because the only durable identity is the
  worktree itself, a lost or not-yet-created workspace means a lost agent. It is
  legitimate for an agent to exist with no workspace, or to need its workspace
  rebuilt — today that case is unrepresentable.

We need a durable index of *what agents exist*, kept outside any single
workspace, that the rest of the system aligns against instead of playing
detective.

## What Changes

- **`.sindri/` project directory** (gitignored, analog to `.todos/`) holds the
  index. It lives in the main repo precisely so it survives workspace loss.
- **`sindri init`** — an interactive command that scaffolds `.sindri/`, writes
  `.sindri/config.json`, and ensures `.sindri/` is gitignored (idempotent — it
  is already ignored in this repo; init adds it on fresh projects).
- **TUI ensures the scaffold at startup** — mirroring the existing
  `container.Ensure()` pattern, the TUI runs the init routine before launching
  if `.sindri/` is absent.
- **`.sindri/agents/<name>.json`** — one file per agent, the durable identity
  record: `name`, `role`, `mode`, `base`, `workspace` (path, may be empty),
  `created_at`. The host writes these at launch; the directory listing *is* the
  roster.
- **Two-layer split, no redundancy.** The index holds identity only. Live
  progress (`task`, `status`) stays in the per-workspace `.sindri-task` files
  the agent already owns. The index points at a workspace; the workspace holds
  what's happening in it.
- **`List()` becomes a reconciler.** The index is the roll call; podman/git/
  workspace files are attendance. Mismatches become actionable status —
  *healthy*, *crashed mid-task*, *idle*, *no workspace (rebuildable)*, *orphan*
  — instead of one undifferentiated "exited".

## Non-goals

- Changing how roles are launched or how binaries are mounted — see the sibling
  changes `role-driven-launch` and `hot-swap-agent-tooling`. This change only
  establishes the index and the reconciler that reads it.
- Auto-rebuilding lost workspaces. This change makes the "no workspace" state
  *representable and visible*; acting on it is follow-up work.

## Impact

- Affected specs: `04-workers` (agent index as source of truth; reconciled
  mapping), `view-workers` (reconciled status shown in the TUI).
- Affected code: `internal/worker/worker.go` (`List` → reconciler),
  `internal/worker/lifecycle.go` (write index entry on launch),
  `cmd/sindri/` (new `init` command; TUI startup ensure),
  a new `internal/sindri` (or similar) package for the `.sindri` scaffold +
  index read/write.
- Migration: the per-workspace `.sindri-task` contract is unchanged; the index
  is additive. Position-based role inference and the `_reviewer` magic are
  retired here (role read from the index) — `role-driven-launch` depends on it.
