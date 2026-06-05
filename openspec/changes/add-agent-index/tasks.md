# Tasks

## 1. `.sindri` scaffold + index package

- [x] 1.1 New package (e.g. `internal/sindri`) with the index types: agent
      identity record (`name`, `role`, `mode`, `base`, `workspace`, `created_at`)
      and read/write of `.sindri/agents/<name>.json`
- [x] 1.2 `EnsureSindri(projectRoot)` — create `.sindri/`, write
      `config.json`, append `.sindri/` to `.gitignore` if missing (idempotent)
- [x] 1.3 Roster read: list `.sindri/agents/*.json` as the canonical set of agents

## 2. `sindri init`

- [x] 2.1 Add `init` command to the host CLI command tree (`cmd/sindri/`)
- [x] 2.2 Interactive flow: confirm project root, create scaffold, report what
      was written; safe to re-run
- [x] 2.3 TUI startup calls `EnsureSindri` before launching; runs init if absent

## 3. Index writes on launch

- [x] 3.1 `worker.Start` writes/updates the launching agent's index entry
      (identity fields only — never `task`/`status`)
- [x] 3.2 Leave the per-workspace `.sindri-task` write/remove path untouched

## 4. Reconciler

- [x] 4.1 Rewrite `worker.List` to start from the index roster, then attach live
      state by joining podman, git branch, workspace `.sindri-task`, and the PR store
- [x] 4.2 Derive reconciled status: healthy / crashed-mid-task / idle /
      no-workspace / orphan (container or worktree with no index entry)
- [x] 4.3 Remove position-based role inference and the `_reviewer` special case
      from `List` (role now comes from the index entry)

## 5. Prune orphans

- [x] 5.1 `worker.Orphans(projectRoot)` — containers/worktrees with no index entry
- [x] 5.2 `worker.RemoveOrphan` — remove the container and delete the worktree
- [x] 5.3 `sindri worker prune` — list orphans, delete only after user confirmation

## 6. Validation

- [x] 6.1 `openspec validate add-agent-index --strict` passes
- [x] 6.2 `go test ./...` green; `sindri lint all` green
- [x] 6.3 Manual: kill a worker container mid-task → `sindri worker list` shows
      "crashed", not "exited"; delete a worktree → shows "no workspace";
      `sindri worker prune` removes a container/worktree with no index entry
