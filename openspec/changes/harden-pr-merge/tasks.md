# Tasks

The code shipped in this session; these record the work and the spec deltas.

## 1. Auto-rebase before merge

- [x] 1.1 Add `git.RebaseOnto(dir, branch, onto)` (checkout + rebase, aborting and
      reporting on conflict so the worktree is left clean).
- [x] 1.2 `Hub.Merge` rebases the PR branch onto the current base before merging;
      a clean rebase proceeds to merge.
- [x] 1.3 On rebase conflict, route the PR back to the owning worker via
      `reject(..., byUser=false)` with the conflict reported, and stop the merge.

## 2. Isolate agents from td

- [x] 2.1 Drop `td` from the agent image: remove it from the build-context staging
      (`internal/container/image.go`) and the `COPY` in `container/Dockerfile`
      (keep `yq`).
- [x] 2.2 Keep `git.CommitAll` honest (`git add -A`) — no silent `.todos`
      exclusion; with no td in the worktree nothing churns it.

## 3. Gitignore .todos/ (close the collision at the root)

- [x] 3.1 Add `.todos/` to `hubIgnores` so `ensureGitignore` writes it into every
      served repo's `.gitignore`; flip `TestEnsureGitignore` to expect it.
- [x] 3.2 Untrack this repo's already-committed `.todos/` (`git rm -r --cached`),
      files kept on disk; drop the stale `.sindri*`/`.sidecar*` `.gitignore` lines.

## 4. Author the delta specs

- [x] 4.1 `03-gh-local`: merge rebases onto base first; clean → merge, conflict →
      back to worker.
- [x] 4.2 `05-workflow`: merge-conflict returns work to the owning worker.
- [x] 4.3 `04-workers`: no direct task-tracker access in the pod (no `td`).
- [x] 4.4 `hub`: `.todos/` is gitignored, never committed.

## 5. Verify

- [x] 5.1 `go test ./...` and `sindri lint all` (deadcode/loc/openspec) pass.
- [ ] 5.2 Manual: submit a PR, advance base under it, merge → confirm it
      auto-rebases and lands; force a conflicting change → confirm it returns to
      the worker with the conflict reported.
