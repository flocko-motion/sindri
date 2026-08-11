# Tasks

## 1. The invariant, first

- [x] 1.1 `plannerMayRate` expresses it as one boundary: move freely, rate the ungated freely, never
      cross unrated/rated on a task the gate would let out.
- [x] 1.2 `authorisedForClaim` mirrors the claim queries' own clause, so a task with NO approval row
      counts as authorised — the case a five-case reading of the rule would miss.

## 2. The verbs

- [x] 2.1 `create-task --priority`, parsed strictly: an unknown word is refused, not guessed at.
- [x] 2.2 `prioritise-task <id> <level>`, one task at a time, registered for planners only.
- [x] 2.3 Refuse with the reason, and write nothing when refusing.
- [x] 2.4 No worker nudge: a planner's rating never makes a task claimable, so one would always be
      false.

## 3. Ordering of writes

- [x] 3.1 In `create-task`, the approval row before the priority — a rated task with no approval row
      is claimable, and the window between them would be real (the hub serves several agents at
      once, so a claim could land inside it).
- [ ] 3.2 NOT PINNED BY A TEST, deliberately reported rather than implied: the window is transient,
      and a final-state assertion cannot observe it — reversing the order leaves an identical task
      row, so 6.3 passes either way. Mutation-checking proved that, not the ordering. Pinning it
      would need a seam that observes the store mid-call; the code is correct by construction and
      the reason is in the comment at the site.

## 4. Consequences named in the task

- [x] 4.1 Reword the `create-task` help: approval releases work, a priority proposes an order.
- [x] 4.2 The human approve surfaces name the priority being authorised (TUI confirm note, CLI
      output).
- [x] 4.3 Verify sd-5c1faf's approve-after-rating confirm cannot fire for a planner. It cannot:
      both halves are front-end code and a planner never reaches them.

## 5. One vocabulary

- [x] 5.1 Move the priority words and their codes into `internal/api`, since the hub now parses them
      and the core may not import a `ui` package. `theme` delegates both directions.

## 6. Pin it

- [x] 6.1 The invariant asserted against CLAIMABILITY itself across all seven states, not against the
      predicate — a predicate test would pass even if the verb consulted it wrongly.
- [x] 6.2 That an allowed rating actually lands, so the invariant is not satisfied by refusing
      everything.
- [x] 6.3 A rated proposal is never claimable.
- [x] 6.4 The reply distinguishes ordering from releasing; unknown words and malformed calls answer
      on the agent's own stream.
- [x] 6.5 Replace the test asserting a planner may set no priority at all — it encoded the rule this
      change reverses, and says so.
