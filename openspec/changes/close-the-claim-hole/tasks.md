# Tasks

## 1. The invariant, and the test first

- [x] 1.1 Write `TestEveryOpenRatedUngatedUnheldTaskIsClaimable` against the CURRENT
      code and confirm it fails on the stranded-package shapes.
- [x] 1.2 Keep `TestAnEmptyPackageIsNotClaimable`'s gated-child exclusion passing
      unchanged — the invariant is additive, not a relaxation of that rule.

## 2. The fix

- [x] 2.1 `store.OpenContainers`: relax the SQL pre-filter to "has a child at all";
      add `HasOpenDescendant` and use it to include a package with nothing left
      anywhere under it, while still excluding one whose only work is gated.
- [x] 2.2 `workflow.claimContainer`: hold a package with zero open subtasks (reusing
      its branch via `git.EnsureBranch`) and return `DirContainerDone` instead of
      declining.
- [x] 2.3 Confirm the invariant test now passes, and add an end-to-end
      `AgentDirective` test for the stranded-package claim.

## 3. Housekeeping

- [x] 3.1 Split `store/workflow.go`'s task read model and claim queries into
      `store/tasks.go` (the file had grown past the 700-line limit).

## 4. Verify

- [x] 4.1 `make verify` passes.
- [x] 4.2 `openspec validate --all` passes.
