# Hub-assisted merge-conflict resolution (worker resolves content, hub drives git)

## Why

When a local PR's branch conflicts with its base, the merge path bounces the PR
back to the owning worker to "resolve against the latest base and resubmit" — but
the worker **cannot**: git is entirely host-side (the worker's `.git` points at a
host path not mounted into its pod), and the hub `--abort`s the rebase, so the
worktree is left clean with nothing to resolve. A plain resubmit re-runs the
identical rebase and re-hits the identical conflict. The multi-agent workflow is
blocked on any branch that has fallen behind or carries a commit the base already
resolved differently (issue #27).

The division of labour was wrong. The worker *can* do the one thing it's good at —
resolve conflict **content** (its `/workspace` files are mounted and editable; only
`.git` is host-side). The hub *must* do the one thing only it can — drive git. So
the hub should assist the worker: run the git mechanics host-side, surface the
conflicts into the worktree, and let the worker iterate until the branch is
mergeable — then the normal review → human-merge flow resumes, now conflict-free.

## What Changes

- **A worker can ask the hub to test mergeability, as often as it wants.** A new
  worker verb (e.g. `sindri rebase`/`mergecheck`) asks the hub to bring the branch
  up to its base. The hub runs the rebase host-side and reports the outcome:
  already-current, cleanly rebased, or conflicted — naming the conflicting files.
- **On conflict, the hub sets the worker up to fix content — it does not abort.**
  The hub leaves the rebase in progress with conflict markers in the worker's
  `/workspace`, and tells the worker which files to resolve. The worker edits them
  (deleting markers / choosing content — including simply keeping base's version
  for a commit the base already superseded) and asks again.
- **The hub continues the git on each ask.** On the next mergeability request the
  hub stages the resolved files and continues the rebase host-side, looping
  (surface → resolve → continue) until the branch applies cleanly onto base. The
  worker never touches git.
- **Once mergeable, the local PR is renewed.** A clean rebase refreshes the PR
  (re-commits the rebased branch, marks it for re-review), the reviewer re-reviews,
  and the human merges — the merge is now guaranteed conflict-free.
- **The merge path stops bouncing conflicts to the worker as a rejection.** A
  merge-time rebase conflict routes into this resolution loop (worker asked to
  resolve), not a dead-end "resubmit" rejection. A human can still resolve
  host-side as the fallback.

## Capabilities

### New Capabilities
<!-- none — this changes the behaviour of the existing hub PR/merge capability -->

### Modified Capabilities
- `hub`: the PR/merge lifecycle gains a worker-driven, hub-assisted mergeability
  loop — the worker requests re-tests and resolves conflict content in its
  worktree while the hub performs all git operations; a branch reaches the human
  merge only once it applies cleanly onto base.

## Impact

- **`internal/adapter/git`**: rebase that leaves conflicts in place (no `--abort`)
  and reports conflicting files; `RebaseContinue`; rebase-in-progress detection;
  optionally `RebaseSkip` for a commit already in base.
- **`internal/hub`**: a new `resolve`/`mergecheck` worker verb (`commands.go`), the
  loop + a "resolving" agent/PR phase and guidance messages (`workflow_pr.go`,
  `prompts.go`), and the merge path (`Merge`) routing conflicts into the loop
  instead of `reject`.
- **UIs** (`cmd/sindri`, `internal/tui`): surface the "resolving" PR/agent phase so
  a human can see a branch is mid-resolution (and step in host-side if needed).
- No change to the on-disk PR store schema beyond a new status/phase value; no wire
  format change for existing endpoints.
