# Tasks

## 1. The `_global` project exists

- [ ] 1.1 Register `_global` at hub start, path = the state dir, so `projectRoot` resolves it and
      every project-keyed path follows the existing layout.
- [ ] 1.2 Refuse it where a command asks which repo to work in: it takes no worktrees and is not a
      repo to act on.
- [ ] 1.3 Pin the naming reason in a test: `repoSlug` strips `$` and `*` to `global`, so a sigil form
      would collide with a repo directory of that name in the container name; `_` survives.

## 2. Global agents are visible everywhere

- [ ] 2.1 `_global` agents are in scope in every project, in both front-ends — never a foreign row.
- [ ] 2.2 They count in the same tallies as local agents, so a section badge cannot disagree with the
      rows beneath it.
- [ ] 2.3 `_global` reads as not-a-repo wherever a tag is displayed.

## 3. A reviewer can live in `_global`

- [ ] 3.1 A reviewer may be created there; workers, planners and coauthors may not, and the refusal
      says why the role differs.
- [ ] 3.2 A repo may still have its own reviewer, and the two coexist with no special case.

## 4. The workspace is materialised, not worktree'd

- [ ] 4.1 A global reviewer's `/workspace` is a fixed host path under its own home.
- [ ] 4.2 Assigning a review materialises the PR's tree there, replacing what the last review left.
- [ ] 4.3 The pod is not restarted to serve a different repo — the mount is fixed, the contents move.
- [ ] 4.4 A failed materialise tells the reviewer `/workspace` does not hold the PR and to review from
      the diff alone, preserving the existing guarantee.
- [ ] 4.5 A global agent's pod mounts no repository; the worktree and scratch mounts are absent.

## 5. Clear at the verdict

- [ ] 5.1 Recording a verdict clears the reviewer's session — not compacts, and not at the next
      assignment.
- [ ] 5.2 The clear re-serves the reviewer's directive, so it is never left waiting to be told what
      to do.
- [ ] 5.3 A test that a reviewer's session carries nothing from the previous review into the next.

## 6. The pool spans projects

- [ ] 6.1 `idleReviewer` and `AssignPendingReviews` consider `_global` reviewers alongside the
      project's own, preferring a local one where both are free.
- [ ] 6.2 "What is this reviewer reviewing" is answered fleet-wide, so a global reviewer's held review
      is found wherever it lives.
- [ ] 6.3 `assignReview` resolves the reviewer's workspace from the reviewer's own record rather than
      the PR's project — today it looks the reviewer up in that project's roster and fails loudly,
      which would fire on every review.
- [ ] 6.4 A review with no free reviewer waits and is handed out when one frees, and stays visible as
      waiting meanwhile.

## 7. Prove the ceiling moved

- [ ] 7.1 A test that reviewer memory is bounded by the pool rather than by the number of repos —
      the property this change exists for.
