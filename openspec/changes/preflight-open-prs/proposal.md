# Keeping an open PR honest as its base moves

## Why

Nothing today tells anyone that an open PR has gone stale. The base moves, the PR sits, and the
first sign of trouble is the human merge. Two separate questions go unanswered:

- Would it still apply? A conflict is discovered at merge time, when the human is already committed
  to landing it.
- Would the combined result work? This is the harder one, and nothing answers it at all. Five
  commits touching a shared test helper can replay without a single conflict and still break what
  lands. The submit gate ran on the branch as it was; no mergeability check runs a gate.

## What changes

Two tiers, hung off the reference-branch sweep that already fires on every move.

- Tier 1 is a cheap filter: is this PR behind its base at all, by a rev-list count. No checkout,
  nothing disturbed. It decides whether to look.
- Tier 2 materialises the combined result in a throwaway worktree — its own branch at the PR tip,
  replayed onto base — and runs the project gate on it. That single operation answers both
  questions: the replay is the applies-verdict, and the gate on what it produced is the other half.

## Decisions the task asked for, and how they went

**The cheap tier does not simulate a merge.** The task offered two routes and asked for a deliberate
choice. I took the first: no `merge-tree`, anywhere. The hub merges by replaying (`03-gh-local`), and
`settleRebase` exists because replay behaves unlike merging — it skips commits the base already
contains, which is exactly the gh-27 case where a redundant commit conflicted on replay while its
content was already in base. A merge simulation would have called that PR clean and left the real
rebase to stop on it. So tier 1 only asks "behind at all", and tier 2's real rebase is the authority.
A pinned test covers that case specifically.

This also removes the git 2.38 dependency the task flagged for graceful degradation. There is no
version-gated command in the change, so there is nothing to degrade around — the concern is answered
by not incurring it rather than by handling it.

**Advisory, not auto-reject.** The finding is recorded on the PR and nothing else happens. Rejecting
routes a PR back to its author mid-review, and on a busy base invites move → reject → rebase →
resubmit → move. The author is also the one party who cannot act while the branch is under review.
I did not wake the worker either, even once its branch is free: for an open PR the author is
essentially always in `submitted`, so that path would fire almost never while adding a second way to
interrupt someone. The human reading the PR is the audience.

**Bounded.** Only PRs actually behind; one PR per sweep; a mutex so two sweeps cannot overlap; and a
memo keyed on the base and branch tips, so a burst of merges is answered once at the resulting state
rather than once per merge.

## Impact

- Specs: `hub` gains the requirement. Complementary to `sd-9f2cad`, which stops a PR being CREATED
  on a stale base — that closes the window before submission, this one the window during review.
- Code: `internal/adapter/git` (`WorktreeAddOnBranch`, `RebaseHere`), `internal/hub/repo`
  (`MaterializeCombined`/`RemoveCombined`), `internal/hub/workflow/prcheck.go` (the tiers and the
  bounding), and one call from `internal/hub/refwatch.go`.
- `RebaseHere` reuses `settleRebase`, so the check and the merge share the one definition of what a
  replay does. A second implementation is how the two would come to disagree.
- The throwaway is one reserved name, `.worktrees/precheck`, cleared before use as well as after, so
  a hub killed mid-check leaves at most one stale tree and the next check clears it.
