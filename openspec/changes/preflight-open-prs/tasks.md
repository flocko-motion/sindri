# Tasks

## 1. Model the operation the hub actually performs

- [x] 1.1 `git.RebaseHere` replays in place and reports conflicts, reusing `settleRebase` so the
      check and the merge cannot disagree about what a replay does.
- [x] 1.2 No `merge-tree`: a merge simulation answers a different question, and would call the gh-27
      case clean. Pinned by a test where a commit's content is already in base.
- [x] 1.3 `git.WorktreeAddOnBranch` gives the throwaway a branch of its own, since the branch under
      test is held by its author's worktree and git will not check it out twice.

## 1b. Onto the base the merge will use

- [x] 1.4 Take the base per PR from `pr.Base`, falling back to the project reference for older rows,
      and thread it through the behind-count, the materialisation and the finding text.
- [x] 1.5 Carry the base name in the memo, so a PR re-aimed at another branch is re-checked even
      when both branches sit on the same commit.

## 2. Tier 1: the cheap filter

- [x] 2.1 Behind-at-all by rev-list count, no checkout, nothing disturbed.
- [x] 2.2 Skip a PR that is level, merged, scrapped, or whose branch is gone.

## 3. Tier 2: what would actually land

- [x] 3.1 `repo.MaterializeCombined` builds the combined result in a reserved throwaway worktree and
      returns the conflicting paths.
- [x] 3.2 Run the project gate on it — the built-in linters plus the configured verify.
- [x] 3.3 Never the author's worktree, and clean up either way. Cleared before use too, so a crash
      leaves at most one stale tree.

## 4. Advisory, with evidence

- [x] 4.1 Record on the PR via its own history; no status change, no rejection, no interruption.
- [x] 4.2 Name the conflicting paths, or quote the gate output (tail-trimmed: a failing run says
      what failed at the end).
- [x] 4.3 A check that could not run is recorded as that, never as a finding about the PR.

## 5. Bound the cost

- [x] 5.1 One at a time, one PR per sweep.
- [x] 5.2 Memo on the base and branch tips, so a burst of merges is answered once.
- [x] 5.3 Run off the drift loop: a gate run must not stall reference detection, nor make a later
      project wait behind an earlier project's gate.

## 6. Pin it

- [x] 6.1 Applies cleanly, conflicts with paths named, and the base-already-has-it skip.
- [x] 6.2 The author's tree and branch unmoved; the throwaway removed; a stale one recovered from.
- [x] 6.3 Level PR unchecked, merged PR unchecked, advisory (no status/feedback/injection), and the
      debounce including that a further move IS checked.
- [x] 6.4 A PR based on a branch that is not the project reference: the conflict with its OWN base
      is found, and the finding names that base and not the reference.
- [x] 6.5 Mutation-checked compilably: defeating the memo fails the debounce test, pointing the
      rebase at the author's tree fails the untouched-tree test, ignoring `pr.Base` fails both
      per-PR-base tests, and dropping the base name from the memo key fails the same-tip re-aim
      test — which the first re-aim test did NOT catch, since it differed by tip as well.
