# Add the container-task workflow: checkpoints + milestone PRs

## Why

Today's worker loop is deliberately rigid: an agent auto-claims one leaf task,
branches on it, submits, **blocks** on review, merges, then takes the next. That
is right for independent one-off tasks, but a feature that decomposes into
subtasks wants a different shape — one branch, many subtasks landed back-to-back,
reviewed and merged as a cohesive whole rather than as N fragmented PRs.

The same shape serves two usage patterns with **no change in mechanism**:

- **Bulk** — "implement this feature and its children, submit it as one PR." The
  children are pre-filled; the agent works the whole list autonomously.
- **Interactive** — "work with me": a human feeds subtasks and asks for a PR at
  milestone moments.

The only difference between them is *who supplies the subtasks and when the PR
fires*. This is a second workflow alongside the structured one — different, not
opposite: same agent, same PR-as-merge-intent with git hub-side, same human-only
merge.

## What Changes

- **Auto-assignment takes only leaves.** A task with children is never
  auto-claimed (today `claimNext` could wrongly grab a parent). Its leaves are the
  unit of automatic work.
- **A container is marked for collaborative assignment.** Any task with children
  may be marked (e.g. a label). When marked, a free agent takes the whole
  container; its open children become that agent's subtask stream and are reserved
  to it (not auto-claimed by others).
- **The branch is named for the container, not the subtask.** It persists across
  subtasks; the agent's *current subtask* is tracked separately. This is the core
  mechanical change — the existing `branch == task` invariant is dropped.
- **Checkpoints are non-blocking.** Finishing a subtask commits it to the
  container branch, closes that child, and advances to the next open child — the
  agent never blocks on review between subtasks.
- **Milestone PRs (blocking).** A milestone PR captures the container branch's
  current state for the human to review and merge, and **blocks the agent until
  that merge lands** — which keeps the worktree quiet so the merge + rebase are
  safe (the simplification: no merging a branch the agent is still editing). After
  merging, the branch is rebased onto the new base and the agent **resumes the
  same container** — not retired, not freed. Triggered on request (interactive) or
  when the children are exhausted (bulk). The branch is retired and the agent
  freed only when the container itself closes.
- **Review is advisory in this mode.** A reviewer's opinion is delivered as
  feedback when requested; merge is not gated on it. The human still merges, and
  no agent merges its own work.

## Capabilities

### Added Capabilities

- `05-workflow`: leaf-only auto-assignment; collaborative assignment of a marked
  container; non-blocking checkpoints; milestone PRs and advisory review.
- `03-gh-local`: the persistent container branch (decoupled from the current
  subtask) and the milestone merge (lands current state, rebases, does not retire
  the branch).
- `04-workers`: an agent may hold a container plus a rolling current subtask and
  is not blocked between subtasks.

## Impact

- **Source of truth (to change):** `claimNext` (leaf-only + container assignment),
  a new checkpoint verb, `store.AgentState` (decouple `Branch` from the current
  task; add the current-subtask field), `Merge` (a milestone variant), branch
  naming.
- **Reuses what exists:** td `ParentID` (containers/leaves — no schema change);
  the planner's standing-branch + `rebasePlanners` rebase-on-merge pattern;
  human approve+merge (`add-host-pr-approve`); the auto-rebase from
  `harden-pr-merge` (what keeps a long-lived container branch mergeable).
- Additive: the structured leaf workflow is unchanged; this is a second mode
  entered by marking a container.
