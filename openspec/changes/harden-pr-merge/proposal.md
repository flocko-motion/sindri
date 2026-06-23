# Harden PR merge: auto-rebase stale branches, isolate agents from td

## Why

Merging an approved PR fails in two everyday situations, and both surfaced as one
opaque error (`Your local changes … would be overwritten by merge`):

1. **A branch that has merely fallen behind base** can't merge cleanly, even
   though nothing actually conflicts — the base just moved on under it. Today the
   human is left to sort it out manually.
2. **Branches were polluted with task-tracker churn.** The agent container shipped
   the `td` CLI, so when an agent ran `td` in its worktree (e.g. a worker testing
   the td-delete feature) it wrote `.todos/` *inside the worktree*, and the work
   commit (`git add -A`) swept it in. Those committed `.todos/` lines then collide
   with the host checkout's own live `.todos/` edits at merge time. Proven on
   `td-bfd811`: its work commit carried two UTC-stamped `command_usage.jsonl`
   lines written in-container.

`.todos/` is the hub's single source of truth, written by the hub against the main
checkout (`td -w h.root`). A PR branch has no business modifying it. The fix is to
**isolate agents from td entirely** — the architecture already says agents reach
external state only through the hub — and to **bring a stale branch current
automatically** before merging.

## What Changes

- **Merge auto-rebases first.** Merging an approved PR rebases its branch onto the
  current base, then merges. A branch that only fell behind now merges with no
  human step.
- **A real conflict goes back to the worker.** If the rebase conflicts (a genuine
  divergence), the merge stops and the PR is routed back to its owning worker to
  resolve and resubmit — the same return path as a rejection — with the conflict
  reported, never silently swallowed.
- **No `td` in the agent container.** The pod image no longer ships the `td` CLI
  (`yq` is still bundled). Agents read and write task state only through the hub
  over their socket, so nothing in a worktree can write `.todos/` — branches stay
  free of task-tracker churn. `git add -A` stays honest: if `.todos/` ever did
  appear in a worktree, it would surface (a loud merge failure), not be silently
  dropped.

## Capabilities

### Modified Capabilities

- `03-gh-local`: merge into base now rebases the branch onto the current base
  first; a clean rebase merges, a conflicting one returns the PR to its worker.
- `05-workflow`: the task lifecycle gains a return-to-worker path for a merge that
  can't apply because the branch diverged from the advanced base.
- `04-workers`: the agent pod has no direct task-tracker access — `td` is not in
  the image; task state is reached only via the hub, keeping `.todos/` out of
  agent branches.

## Impact

- **Source of truth:** `internal/adapter/git/git.go` (`RebaseOnto`; `CommitAll`
  stays `git add -A`), `internal/hub/workflow_pr.go` (`Merge` rebases then routes
  conflicts back via `reject`), `internal/container/image.go` and
  `container/Dockerfile` (drop `td` from the image).
- **Caveat:** branches created before the isolation fix may still carry a
  `.todos/` commit in their history and can still collide on merge — recreate or
  redo those.
