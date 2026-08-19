# Tasks

## 1. The reviewer pool spans projects (sd-451199)

- [x] 1.1 `idleReviewer` checks a project's own roster first, then `GlobalProject`'s
      (`idleReviewerOn`), preferring local over pooled.
- [x] 1.2 `reviewingPR(home, name)` finds what a reviewer holds and where it is filed — a
      project-bound reviewer's own store directly, a `GlobalProject` reviewer's fleet-wide over
      every known project — so idle-checking a pooled candidate is a fleet-wide question.
- [x] 1.3 `assignReview` resolves the reviewer's own roster row, workspace, state and notes from its
      own home (`reviewerHome`) rather than the PR's project; the review record itself stays with
      the PR's project.

## 2. The `_global` virtual project exists (sd-7374d3)

- [x] 2.1 `_global` registers at hub startup (`hub.New`), not lazily like a repo, with a path whose
      basename is `_global` so `repoSlug`/container naming need no special-casing.
- [x] 2.2 `project.Service.Forget` refuses it — not a repo, and the hub re-registers it on its next
      restart regardless.
- [x] 2.3 `StartupAdvice` skips it — no repo, no architecture doc to advise on.
- [x] 2.4 The naming rationale is pinned in tests: `RepoTag` is hex-only and can never equal
      `"global"`; `repoSlug` keeps underscore but strips `$`/`*` to the same collision `_global` was
      chosen to avoid.

## 3. Global agents are in scope in every repo (sd-808cdc)

- [x] 3.1 `GlobalProject` moves to `internal/api` (wire-level), re-exported as `workflow.GlobalProject`
      for that package's own call sites, so both front-ends can name it without linking hub code.
- [x] 3.2 The TUI's `inScope` (feeding `agentVisible`/`prVisible`/mail scope and every badge that
      reads them) admits `GlobalProject` unconditionally — never foreign, regardless of whether it
      needs the user.
- [x] 3.3 The CLI's `listGroupFor` (agent/PR/mail listings) does the same.
- [x] 3.4 The TUI repo switcher excludes `GlobalProject` — not a repo to switch into, and dialing its
      registered (hashed) path there would register a phantom duplicate project.

## 4. A reviewer's context is cleared when its verdict lands (sd-a19ef9)

- [x] 4.1 `completeReview` fires an unconditional `FireClear` on the reviewer's own project instead
      of delivering a push message — the verdict just given is the review's own leaf boundary.
      `interrupt=false`: this runs inside the reviewer's own request, so nothing of its own is in
      flight to cut off.
- [x] 4.2 `FireClear`'s own re-serve (queuing `MsgKickoff`) replaces the old `MsgVerdictRecorded`
      push, now dead and removed.

## 5. A global reviewer's workspace is materialised per review (sd-d06eff)

- [x] 5.1 `git.ArchiveTree` exports a ref's tree into a destination as plain files, no `.git`,
      clearing the destination's prior contents first.
- [x] 5.2 `assignReview` calls it for a `GlobalProject` reviewer instead of `CheckoutDetachedClean`,
      reusing the existing `checkedOut=false` failure path (the reviewer is told not to trust
      `/workspace` and to read the diff alone) when the materialise fails.
- [x] 5.3 `Launch`'s git/worktree setup is extracted into `prepareWorkspace`, which skips
      `HasCommits`/`WorktreeAdd` entirely for a `GlobalProject` pod (there is no repository) and just
      creates the fixed workspace directory.
- [x] 5.4 No mount-table change needed: a reviewer's existing mount is already `/workspace` alone —
      no worktree or scratch mount, matching "a `_global` pod mounts no repository at all".

## 6. A reviewer can be created in the global pool (sd-e7cced)

- [x] 6.1 `NewAgent` refuses every role but reviewer in `GlobalProject`, naming what the refused role
      holds across its work (a branch, a standing conversation, the user's own seat).
- [x] 6.2 `reqProject` resolves the literal `_global` header as the tag it already is, instead of
      hashing it as a path — which would have silently registered a phantom project.
- [x] 6.3 The CLI gets `agent new --global` (`withGlobalBackend`, dialing `GlobalProject` directly,
      bypassing `repoRoot`); the TUI's new-agent picker gets a "reviewer (global)" choice that dials
      a one-off client to `GlobalProject` rather than touching the ambient client.

## 7. A global reviewer follows its review past assignment (review fix)

- [x] 7.1 `reviewDirective` resolves where a held review is actually filed (`reviewingPR`) before
      reading/closing it, rather than assuming its own project — a pooled reviewer asking for its
      directive now finds the review it holds instead of `DirNoReviews`.
- [x] 7.2 `openPR`, `CmdShowPR` and `CmdLint`'s PR-argument path (`lintPR`) all resolve a named PR's
      actual project before touching its store: the PR's own project for the reviewer holding it
      fleet-wide, the caller's own project for every other caller (-> `callerPRProject`, round 3).
- [x] 7.3 `CmdApprove`/`CmdReject`/`completeReview` split the PR's project (record, status, log) from
      the reviewer's own home (roster, state, `FireClear`) — the two are the same project for every
      role but a pooled reviewer, and were silently conflated before.
- [x] 7.4 Tests follow a pooled reviewer past assignment: its own directive, an approve and a reject
      across projects, and reading the PR it holds.

## 8. Every reader of "what does this reviewer hold" agrees (review fix, round 2)

- [x] 8.1 `store.Store.ReviewingPR`/`RuledPRs` (`internal/hub/store/reviewpool.go`) replace
      `workflow.Engine`'s own `reviewingPR`: home's own project first, else fleet-wide — safe because
      an agent's name is unique across the whole fleet, so a hit elsewhere can never belong to a
      different agent. Reachable from `package agent`, `package hub` and `package workflow` alike,
      which a method on `Engine` was not.
- [x] 8.2 Every direct, project-scoped `ReviewingPR`/`RuledPRs` read that a pooled reviewer could
      reach now goes through it instead: `agent.Service.AtLeafBoundary` (safety-critical — a
      project-scoped read let `/clear` fire mid-review), `agent.Service.HoldsNothing`,
      `hub.reviewerTasks` (the comment-scope gate), the board's PR column, the stall nudge, the
      "why nothing is assigned" explanation, and `staffHolding`.
- [x] 8.3 `freeReviewer` falls back to `GlobalProject` too, mirroring `idleReviewer` — a submit
      reaches the pool directly (`RequestReview`), not only through the periodic
      `AssignPendingReviews` tick.

## 9. Resolving a named PR never crosses projects for anyone but the reviewer holding it (review fix, round 3)

- [x] 9.1 `callerPRProject` widens beyond the caller's own project only when `store.ReviewingPR`
      confirms the caller itself holds that exact PR fleet-wide; every other caller stays scoped to
      its own project, exactly as before a reviewer pool existed.
- [x] 9.2 `CmdShowPR` and `CmdLint`'s PR-argument path use it (already fixed in round 2's own
      follow-up); `openPR` (now takes the caller, reached by `CmdApprove`) and `CmdReject` do too —
      both had widened unconditionally via `PRProject`, letting any reviewer/planner/coauthor resolve
      and act on a PR belonging to a project it holds nothing in.
- [x] 9.3 `stampVerdict` takes the caller's already-resolved project instead of re-deriving it from a
      bare id, so the safety `openPR`'s gate established is not undone one call later.
- [x] 9.4 A regression test: an unrelated worker naming a foreign PR id is refused, with nothing of
      that PR's data in the output.
