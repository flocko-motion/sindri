## Context

Today's flow (`internal/hub/workflow_pr.go`):

- `cmdSubmit`: lint → `git.CommitAll(worktree)` → create PR (`status: "open"`, id
  `pr-<task>`, branch, base) → agent phase `submitted` → notify reviewers.
- reviewer approves → `status: "approved"`.
- human `Merge`: `git.RebaseOnto(worktree, branch, base)` then `git.Merge(root,
  base, branch)`. On rebase conflict it calls `h.reject(...)` — bounces to the
  worker as a rejection with a text message.
- `git.RebaseOnto`/`Rebase` (`internal/adapter/git/git.go`) run `git rebase` and on
  conflict `git rebase --abort`, returning the error — leaving the worktree clean.

The break (issue #27): the worker can't act on the bounce. Git is host-side (the
pod's `.git` points at `…/.git/worktrees/<agent>`, not mounted), and the aborted
rebase leaves nothing to resolve. It's also often a *history* conflict (a commit
the base already superseded), which no resubmit changes.

The insight: the worker's `/workspace` **is** mounted and editable — only `.git`
isn't. So the worker can resolve conflict *content*; the hub must run git.

## Goals / Non-Goals

**Goals:**
- A worker-driven, idempotent mergeability loop: ask → hub reports/sets up → worker
  edits content → ask again → hub continues → … → clean.
- The hub performs all git; the worker only edits files; the worker is never told
  to run git or operator commands.
- A branch reaches the human merge only once it applies cleanly onto base; the
  merge-time conflict path feeds the loop instead of a dead-end rejection.
- Preserve the linear-history intent (rebase, not merge commits on the branch).

**Non-Goals:**
- Changing the split-brain read/write model (direct-sqlite reads for speed, `td`
  CLI writes for conformance stay).
- Auto-merging or auto-resolving genuine content conflicts for the worker — the
  hub sets them up; the worker (or a human) decides the content.
- Removing the human merge gate or the reviewer step.

## Decisions

- **New worker verb `resolve`** (name TBD — `resolve`/`mergecheck`/`rebase`), shown
  when the agent holds a branch. It is the single entry point the worker calls
  repeatedly. Semantics: "make my branch current with base; if that needs content
  decisions, set them up for me." Idempotent — safe to call any number of times.
- **git adapter gains a non-aborting rebase.** Split `Rebase` into the existing
  abort-on-conflict form (kept for planners/best-effort callers) and a new
  interactive form used by the loop:
  - `RebaseStart(dir, branch, onto) (conflictedFiles []string, done bool, err error)`
    — checkout branch, `git rebase onto`; on conflict return the unmerged files
    (`git diff --name-only --diff-filter=U`) and leave the rebase in progress.
  - `RebaseContinue(dir) (conflictedFiles []string, done bool, err error)` —
    `git add -A` then `git -c core.editor=true rebase --continue`; returns the next
    conflict set or done.
  - `RebaseInProgress(dir) bool` — presence of `.git` rebase-merge/rebase-apply
    state, so `resolve` knows whether to start or continue.
  - Consider `RebaseSkip(dir)` for a commit that becomes empty against base
    (already applied), so redundant commits drop without worker action.
- **The `resolve` handler** (in `workflow_pr.go`): if no rebase in progress →
  `RebaseStart`; else `RebaseContinue`. If conflicts remain, set the agent phase to
  `resolving`, log, and inject a message naming the files. If done, rebase is clean
  → renew the PR: re-commit is already in place (rebase rewrote the branch), set PR
  back to `open` and re-notify reviewers, agent phase `submitted`.
- **Merge path change**: on the pre-merge `RebaseOnto` conflict, instead of
  `h.reject`, drive the same loop — put the agent into `resolving` and inject the
  file list — and return a message to the human that the branch went to its worker
  to resolve (with the host-side fallback still available).
- **New phase/status `resolving`** on the agent state (and reflected on the PR), so
  the command surface can show `resolve` and the UIs can render the state.
- **Messages** (`prompts.go`): a "here are the conflicts, edit these files then run
  `resolve`" directive, and a "clean — back to review" reply.

## Risks / Trade-offs

- **A left-in-progress rebase is stateful on disk.** If the loop is abandoned, the
  worktree sits mid-rebase. Mitigate: `resolve` detects an in-progress rebase and
  continues it; add an explicit abort path (verb or human action) and detect/repair
  a stale rebase on the next `submit`.
- **Per-commit replay can surface the same file across several commits.** The worker
  may resolve more than once. Acceptable; a future `RebaseSkip`/`--onto` refinement
  can cut redundant replays.
- **`rebase --continue` wants an editor.** Force a non-interactive editor
  (`core.editor=true` / `GIT_EDITOR=true`) so the hub never blocks.
- **Concurrent hub git ops in one repo.** Rebasing a worker's branch touches its
  worktree only, but the shared object store is one repo — keep git calls
  serialized per repo as they are today.
- **Genuinely unresolvable-by-agent conflicts** (deep history divergence): the human
  fallback stays — a human resolves host-side and the loop picks up from clean.
