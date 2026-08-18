# Tasks

## 1. Arm rather than refuse

- [x] 1.1 `SetClearArmed` records the arming and fires at once for an agent already at a boundary.
- [x] 1.2 `AtLeafBoundary`: no leaf task, no verdict owed — a held feature is not such a thing.
- [x] 1.3 The fire re-checks the boundary, so nothing clears an agent that picked work up meanwhile.
- [x] 1.4 It runs off the hub's tick, never the agent's request, which is a turn in flight.

## 2. Hold the boundary open

- [x] 2.1 `claimNext`, the feature's advance and BOTH reviewer paths (`freeReviewer`, `idleReviewer`)
      hand an armed agent nothing — the request side as well as the sweep.
- [x] 2.2 `DirClearPending` tells it why, so it does not read an empty queue as "nothing to do".
- [x] 2.3 The arming outranks the fullness notice, which asks for the very act already taken.
- [x] 2.4 A checkpoint with a clear armed holds the next subtask rather than advancing onto it.

## 3. Durable, and visible

- [x] 3.1 An `agents.clear_armed` column, migrated in beside `retired`.
- [x] 3.2 `AgentView.ClearArmed` on the board; a marker on the TUI row and in `sindri agent list`.
- [x] 3.3 The detail says it is armed and when it lands, and that the same key cancels.

## 4. The toggle

- [x] 4.1 The modal confirms and states when, from `api.ClearWaitsFor` — one rule, both front-ends.
- [x] 4.2 The key again disarms with no modal; the CLI's `--cancel` is the same operation.

## 5. Pin it

- [x] 5.1 Arming a working agent succeeds and waits; the flag is read back from the store.
- [x] 5.2 The boundary: mid-subtask and mid-review are not one, between subtasks is.
- [x] 5.3 The gate withholds work, and gives it back the moment the arming is taken away.
- [x] 5.4 The armed answer outranks the fullness answer.
- [x] 5.5 The sweep passes over an agent still working.
- [x] 5.6 An armed reviewer is not assignable, and is again once disarmed.
- [x] 5.7 A failed immediate clear leaves no arming behind — the flag is read back, not assumed.
- [x] 5.8 The confirm states when for each state; the second press asks nothing; both markers show.
