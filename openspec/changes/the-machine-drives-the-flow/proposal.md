## Why

The flow machine was built to be the one thing that moves an agent, and half of it
was never wired. `machine.Run` — the router that applies a verb's declared landing
state — has no caller anywhere in the module, tests included, so every transition
written in the four role maps is checked for spelling at boot (`machine.go:146`)
and executed never. Verbs therefore move agents themselves: roughly twenty-eight
`SetState` calls across twelve files, beside the three the machine owns.

`tick_reference.go` labels three of its own steps "A BACKSTOP", which is the
scaffolding of an unfinished migration still holding weight. One of them has
expired: "the state machine does not yet drive [the reviewer half] in the
background" stopped being true when the reviewer's states started running on the
beat, and the backstop became a second driver.

That second driver stranded a reviewer. On 2026-09-10 a PR was filed while every
reviewer's pod was down. `RequestReview` behaved correctly — `freeReviewer`
requires `s.Up`, found nobody, and left the row unclaimed. Twenty-one seconds
later the machine's own beat walked all four stopped reviewers into
`reviewer/clearing` on `cond.ReviewWaiting`, which asks whether a row is waiting
and never whether the reviewer is alive. The clear failed against a dead pod
(`clear-unconfirmed: agent "balin" is not running`), `act.Failed` led to `Taking`
regardless, and balin claimed the review. The brief could not be delivered
(`inject-skipped`) and nothing ever woke it, because both wake paths key on an
*unclaimed* row and the claim had erased the only evidence a reviewer was needed.

Hundreds of tests pass over this. The liveness rules are asserted only through
`AssignPendingReviews`, never through the machine's own path; `newEngine` never
calls `.Reconciling()`, so the beat exists only in production; and
`flowtest.Hub.Alive` defaults to false, so most fixtures already build the
stranded shape and assert it is correct.

## What Changes

- **BREAKING** `machine.Offer.To` and `machine.Run` are deleted. A verb records a
  fact and publishes; the machine's conditions decide the transition. `machine.Would`
  already answers "where will this land" by following the conditions, so the
  declared targets were a second table restating what the map already knows.
- One writer for anything that hands work to an agent. `SetState`, `AssignReview`,
  `CloseReviews`, `ClaimLeaf` and `ClaimContainer` gain a single caller each — the
  machine — enforced by an architecture test with no exemptions, fixtures included.
- The parallel claimers go: `ClaimNext` (reached from `CmdNext`), `ReviewDirective`'s
  claim, and `RequestReview`'s assignment. `doPickWork` and `doTakeReview` remain.
- The parallel session preparers go: `PrepareAssignment` and `FireClearIfArmed`
  fold into the `clearing` and `retiering` states, which already do this work.
- The pod becomes states. `act.Launch` and `act.Stop` join the vocabulary, every
  role gains `launching` and `stopping`, and `FireIdleStops`, `FireIdleStarts`,
  `wakeAReviewer` and `checkStuckLaunch`'s bounding are deleted.
- Every wait the hub performs becomes a state. `pr/merging` is entered and
  `act.Merge` runs inside it, which makes the declared `pr/stuck` reachable and
  retires `ReconcileMergingPRs`. A submit's questionnaire becomes
  `worker/interviewing`, running a cancellable process that reverts to `working`
  when the code moves underneath it.
- Repairs become conditions. `RepairReviewRows`, `HealPlannerTasks` and
  `HealSplitHierarchies` are expressed on the subjects that own them —
  `HealSplitHierarchies` duplicates `cond.TreeSplit` and `act.Yield` outright.
  The preflight sweep is left holding the task-source pollers, which is its
  honest job.
- The coauthor and planner gain the lifecycle states, so `FireArmedClears` goes.
  Lifecycle states are built by factories taking a struct of destinations, so a
  missing edge is a compile error rather than an absent line — the failure that
  produced this change was one such line missing from one of two copies.
- `CmdState`, which lets a planner write its own flow phase, is removed: an agent
  declaring where it stands has no mechanism left.
- `legacy()` goes with the bare phase words its writers stop producing.

## Capabilities

### New Capabilities
- `flow-authority`: who may move an agent, and what an action outside the machine
  may do instead — write a fact and publish it. Covers the single-writer rule, the
  event-as-hint discipline, and the requirement that every wait the hub performs
  is a state.

### Modified Capabilities
- `agent-runtime`: the pod's lifecycle becomes flow states. Starting and stopping
  an agent are transitions the machine performs, and a human's request is a flag
  or a request record the machine acts on.
- `05-workflow`: a submit is an interview the agent sits in, and a merge is a
  state the hub waits in. Both are moments inside verbs today.

## Impact

- **Source of truth:** `internal/hub/flow/machine` (the engine, less its verb
  router), `internal/hub/flow/agent/roles/*` (four maps, gaining lifecycle and
  interview states), `internal/hub/flow/fleet/flowdo.go` (the actions).
- **Deleted:** `machine/verbs.go`'s `Run`, `Offer.To`, `ClaimNext`,
  `PrepareAssignment`, `FireClearIfArmed`, `FireArmedClears`, `FireIdleStops`,
  `FireIdleStarts`, `wakeAReviewer`, `ReconcileMergingPRs`, `RepairReviewRows`,
  `HealPlannerTasks`, `HealSplitHierarchies`, `CmdState`, `legacy()`.
- **Tests:** fixtures reach a state by writing the domain fact and calling `Look`,
  so no test can pin behaviour for a state the hub cannot produce. One test runs
  the real loop and asserts a stopped agent with work waiting ends up started.
- No change to what an agent is told or what a human sees. The board reads the
  same column; the difference is who is allowed to write it.
