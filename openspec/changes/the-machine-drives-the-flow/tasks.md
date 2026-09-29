## 1. Prove the bug first

- [x] 1.1 Add a fleet test with a waiting review and every reviewer's fixture reading
      `Alive: false`, asserting no reviewer is assigned — it fails today, since
      `cond.ReviewWaiting` never asks about liveness
- [x] 1.2 Add a test that a reviewer holding a review with a stopped pod is started,
      asserting the claim never erases the evidence a reviewer was needed
- [x] 1.3 Confirm whether `startable`'s `m.ran[subject+"\x00"+action]` guard resets on
      state entry, and record the answer in `design.md` — it decides whether an action
      can restart after a `Stay`

## 2. Lifecycle states, built once

- [x] 2.1 Add a factory package building each lifecycle state from a struct of
      destinations with required fields, so an omitted edge fails to compile
- [x] 2.2 Build `clearing`, `mail`, `escalated` and `retired` through it for all four
      roles, giving the reviewer's `clearing` the `cond.SessionGone` edge it lacks
- [x] 2.3 Give the coauthor and planner their lifecycle states, so no role is
      unreachable by the machine for a clear
- [x] 2.4 Decide `escalated`'s shape — a factory parameter for its role-specific verb
      list, or hand-written — and apply it consistently

## 3. The pod becomes states

- [x] 3.1 Add `act.Launch` and `act.Stop` to the action vocabulary with their outcomes
- [x] 3.2 Add `launching` and `stopping` states to all four role maps, bounded by their
      own dwell rather than by a sweep
- [x] 3.3 Record a start or stop asked for by a human as a flag or request record, and
      publish; wire `sindri agent start`/`stop` to it
- [x] 3.4 Delete `FireIdleStops`, `FireIdleStarts`, `wakeAReviewer` and `FireArmedClears`
- [x] 3.5 Move `checkStuckLaunch`'s bounding into the launching state and delete it from
      the watchdog sweep
- [x] 3.6 Make 1.1 and 1.2 pass

## 4. Merging becomes a state

- [x] 4.1 Record a human's merge as an intent against the PR and publish it
- [x] 4.2 Give `pr/approved` the transition into `Merging`, and `Merging` its action,
      outcomes and observed exits (a base already carrying the branch; a recorded
      conflict)
- [x] 4.3 Delete `ReconcileMergingPRs`, and add a test that a hub dying mid-merge
      recovers through `pr/stuck`
- [x] 4.4 Stop `Merge` writing `pr.Status` directly

## 5. The submit interview

- [x] 5.1 Add `worker/interviewing` and `act.Interview`, conducting the exchange under
      the machine's cancellable context. An action that waits on its own SUBJECT needed a
      declaration of its own — `machine.Action.Awaits`, which keeps it off a caller's
      goroutine, since `Look` from the agent's own `sindri` would otherwise wait behind
      the answer it is the call for
- [x] 5.2 Re-pose a question when the agent is observed at an empty prompt *since* that
      question was put, using the observation's own stamps. `interviewGrace` covers the lag
      between typing into a pane and that session reporting a turn
- [x] 5.3 Record the working tree when the interview opens (`git.TreeFingerprint`); check it
      on answer arrival and on re-posing, and revert to `working` when it has moved
- [x] 5.4 Route `verb.Submit` from `working` into the interview, leaving `act.Submit`
      unattended in `submitting`. The submit is a recorded REQUEST (`submit_intents`), the
      questions are written down when it opens, and `pr.TakeSubmit` is the commit-and-gate
      the state runs
- [x] 5.5 Test the three exits: every question answered, the tree moved, the interview
      abandoned and visible with its dwell (-> `flowinterview_test.go`, six cases)

## 6. Verbs record facts and announce

- [x] 6.1 Delete `machine.Run` and `Offer.To`; reduce `Offer` to the verb and its reason
- [x] 6.2 Give each action the topic that names what it did, in place of
      `workflowDeps.Notify()`'s blanket `SessionRead`. The observer is now the one publisher of
      `SessionRead`, per agent and only when the reading MOVED; `MailArrived` and `TaskApproved`
      gained the publishers their listeners had been waiting out a poll for. An architecture test
      holds the vocabulary to its own claim (-> `internal/arch/topics_test.go`)
- [x] 6.3 Subscribe `cond.ReviewWaiting` to the topic a filed review now publishes — a new
      `topic.ReviewFiled`, announced to the whole FLEET, since a pooled reviewer whose own repo
      holds no pull requests never heard a project-scoped one
- [x] 6.4 Strip every `SetState` from the `Cmd*` handlers, leaving each to record its
      domain fact and publish. `SetState` is GONE: it wrote what an agent holds and where it
      stands in one call, and those are two facts — `SetHolding` records the first and
      `SetPhase` the second, with the machine the only caller of `SetPhase`
- [x] 6.5 Delete `CmdState` — a planner's own resting state is derived from its session now
- [x] 6.5b Delete `legacy()`; it stands until 6.4 stops the verbs writing bare phase words
- [x] 6.6 Assert the acceptance criterion: with every `Wake` call suppressed, the suite
      still passes — `SINDRI_DEAF=1 go test ./...`, green, and the one loop test runs deaf

## 7. Parallel claimers and preparers

- [x] 7.1 Delete `ClaimNext` and route `CmdNext` to record intent and publish
- [x] 7.2 Delete `ReviewDirective`'s claim and `RequestReview`'s assignment, leaving
      both to file the row and announce
- [x] 7.3 Fold `PrepareAssignment` and `FireClearIfArmed` into the `clearing` and
      `retiering` states, and delete them
- [x] 7.4 Delete `ReleaseReviewers`' state write and message, leaving the machine's
      `dropping` state as the one release

## 8. Repairs become conditions

- [x] 8.1 Express `RepairReviewRows` as a condition on the PR flow's `filed` state,
      beside `held` and `unheld`
- [x] 8.2 Express `HealPlannerTasks` as a condition on the planner's map
- [x] 8.3 Delete `HealSplitHierarchies`, which duplicates `cond.TreeSplit` and
      `act.Yield`, and confirm the state covers what the sweep did
- [x] 8.4 Reduce `tick_reference.go`'s preflight to the task-source pollers

## 9. The guard

- [x] 9.1 Convert every fixture to reach a state by writing the domain fact and calling
      `Look`. All ~150 go through one door now (`flowtest.Place`), which writes the two facts
      through the two writers and FAILS on a state no role declares; the fixtures whose subject
      is the flow run the machine. Short of the original wording — see the spec's amended
      "A test reaches a state through one door" for what landed and what it does not buy
- [x] 9.2 Add the architecture test asserting one caller for `SetState`, `AssignReview`,
      `CloseReviews`, `ClaimLeaf` and `ClaimContainer`, with no exemptions
      (-> `internal/arch/onewriter_test.go`; `SetState` is now `SetPhase`)
- [x] 9.3 Remove `internal/hub/flowtest/reviews.go` from `arch/situation_test.go`'s
      `ruleDerivers`
- [x] 9.4 Add the one reconciling test that runs the real loop — and it runs DEAF, so only
      the states' own polls can carry it (-> `flowmachine_reconciling_test.go`)
- [x] 9.5 Run `brokkr lint` and make the whole suite green with no allowlist entries
