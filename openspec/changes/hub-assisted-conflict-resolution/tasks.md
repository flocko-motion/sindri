## 1. git adapter: interactive (non-aborting) rebase

- [x] 1.1 Add `RebaseStart(dir, branch, onto) (conflicts []string, done bool, err error)`: checkout branch, `git rebase onto`; on conflict return unmerged files (`git diff --name-only --diff-filter=U`) and leave the rebase in progress (do NOT `--abort`).
- [x] 1.2 Add `RebaseContinue(dir) (conflicts []string, done bool, err error)`: `git add -A` then `git rebase --continue` with a non-interactive editor (`GIT_EDITOR=true`); return the next conflict set or done.
- [x] 1.3 Add `RebaseInProgress(dir) bool` (detect `.git/rebase-merge` / `rebase-apply`).
- [ ] 1.4 (Optional) Add `RebaseSkip(dir)` for a commit already applied in base, and an abort path for a stale/abandoned rebase.
- [x] 1.5 Keep the existing abort-on-conflict `Rebase`/`RebaseOnto` for planner/best-effort callers; add unit tests for the new functions over a temp repo with a synthetic conflict.

## 2. Hub: the worker-driven mergeability loop

- [x] 2.1 Add a `resolve` worker verb in `commands.go` (shown when the agent holds a branch), routing to a new handler.
- [x] 2.2 Handler: if `RebaseInProgress` → `RebaseContinue`, else `RebaseStart`. On remaining conflicts set the agent phase `resolving`, log, and inject a message naming the files. On done, renew the PR (status back to `open`, phase `submitted`, re-notify reviewers).
- [x] 2.3 Add the `resolving` phase/status to agent state (and reflect it on the PR); make the command surface show `resolve` in that phase.
- [x] 2.4 Guidance messages in `prompts.go`: "conflicts in <files> — edit them in your workspace, then run `sindri resolve`" and "clean — back for review".

## 3. Hub: merge path feeds the loop, not a rejection

- [x] 3.1 In `Merge`, replace the `h.reject(...)` on the pre-merge `RebaseOnto` conflict with entry into the resolution loop (agent → `resolving`, inject the file list), and return a human-facing message that the branch went to its worker (host-side fallback still documented).
- [x] 3.2 Ensure a clean loop completion leaves the branch mergeable so the subsequent human `Merge` applies without conflict.

## 4. UIs surface the resolving state

- [x] 4.1 `cmd/sindri`: `pr`/`agent` listings show the `resolving` phase/status.
- [x] 4.2 `internal/tui`: render `resolving` on the agent/PR rows and detail, so a human can see a branch is mid-resolution and step in host-side if needed.

## 5. Verify

- [x] 5.1 `make verify` green (incl. the new git-adapter tests).
- [ ] 5.2 Reproduce issue #27 end-to-end: a branch carrying a base-superseded commit → `resolve` surfaces the conflict in the worktree → resolving the file + `resolve` again rebases clean → PR renews → merge is conflict-free.
- [x] 5.3 Confirm no worker-facing message instructs git/infra commands, and the real error is logged host-side.
