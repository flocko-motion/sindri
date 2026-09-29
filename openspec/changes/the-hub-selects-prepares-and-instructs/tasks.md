# Tasks

## 1. One reading of what an agent runs

- [x] 1.1 `observe.Observation.ModelInUse` joins the recorded choice with the detected one — the
      transcript while the pod is up, the roster row otherwise. `harness.ModelInUse` is deleted.
- [x] 1.2 `situation.Situation.Model` carries that reading, joined once by the gatherer. It shadows
      the embedded observation's raw figure, which stays reachable as `Observation.Model`.
- [x] 1.3 `SetModel` decides "already there" from what the session runs. The roster row is written
      whenever the choice changes; the injection is skipped only when the session is already on it.
- [x] 1.4 `harness.Service.TierIs` and `SetTier` own the whole tier-and-model question.
      `Deps.ModelForTier` and `Harness.ModelMatches` come off `core`; `Service.ModelForTier` is
      deleted with its last caller.
- [x] 1.5 `tierMismatch`, `doRetier` and `TierPrefers` call one of the two. `TierPrefers` stops
      comparing with `==`, which never matched a dated snapshot id against its tier's plain one.

## 2. The chain is linear

- [x] 2.1 `act.HandOver` is declared; `PickWork` and `PickSubtask` select without speaking.
- [x] 2.2 `task.Act.Brief` renders what a worker holds, from its row. `ClaimLeaf`, `ClaimContainer`
      and `StartSubtask` return no directive. `DirContainerClaimed` is deleted.
- [x] 2.3 `task.Act.HeldTier` reads the rating of the unit a worker holds, since the work is no
      longer waiting in the backlog by the time the model step runs.
- [x] 2.4 The worker's states become `assigning | picking → preparing → retiering → handing-over →
      working`. `worker/preparing` and `worker/handing-over` are new; `busyWords` names all three
      steps in the agent's own terms.
- [x] 2.5 `cond.SessionInTheWay` and `cond.TierMismatch` are deleted, with `flow.World.TierMismatch`
      and the gathering behind it. Neither `idle` nor `between-subtasks` reads anything about a
      session.
- [x] 2.6 `worker/clearing` is the human's armed clear alone, resting at `Idle` both ways.
- [x] 2.7 Every step after a selection falls back to `working` when its pod or the hub goes away,
      since the work is held by then.

## 3. A preparation step that does not land stops the agent

- [x] 3.1 `act.Prepare` is its own action beside `act.Clear`: the chain's clear and the human's armed
      clear run the same reset, and a failure means something different in each.
- [x] 3.2 `doPrepare` and `doRetier` escalate through `Deps.Escalate` and return `act.Failed`;
      `worker/preparing` and `worker/retiering` lead to `Escalated` there and nowhere onward.
- [x] 3.3 `prompts.AskPrepareFailed` and `AskRetierFailed` name what was being done and the harness's
      own reason.
- [x] 3.4 `SetTier` refuses a tier the backend does not recognise.

## 3a. The model step types the switch alone

- [x] 3a.1 `chooseModel` is the half `SetModel` and `SetTier` share: validate, record, and report
      whether a live session still has to be told. It types nothing.
- [x] 3a.2 `SetTier` injects `/model` and no clear. `SetModel` keeps its clear for the human's
      standalone change, where nothing has prepared the session.
- [x] 3a.3 `TestTheModelSwitchDoesNotClearASessionJustCleared` holds it. Before the split it did not
      merely fail — it hung for the full `clearSettleCap`, which is the bug: a freshly cleared session
      reports a few tokens, `awaitCleared` waits for a FALL, and a reading at the floor never falls.
- [x] 3a.4 `internal/arch/runtime_test.go`'s declared prober moves from `SetModel` to `chooseModel`,
      which is where the liveness reading now sits.

## 3b. A selection that cannot be made stops the agent too

- [x] 3b.1 `machine`'s action runner lands a NAMED outcome even when the doer also returned an error,
      and skips one whose own run was cancelled. Sixteen `return act.X, err` sites across the flow
      were dead: `doHandOver`, `doPrepare`, `doRetier`, `doRelease` and the rest all parked their
      subject in the acting state, restarted on every poll.
- [x] 3b.2 `TestAnActionsOutcomeLandsEvenWhenItAlsoFailed` holds it, with
      `TestAnActionThatOnlyFailedMovesNobody` for the other half. Proved by restoring the early
      `return` and watching the first fail.
- [x] 3b.3 `act.PickWork` and `act.PickSubtask` gain `Failed`; `assigning` and `picking-subtask` lead
      to `Escalated` there. `fleet.selectionFailed` logs, escalates through `Deps.Escalate` and
      returns it. `prompts.AskClaimFailed` names the unit and the reason.
- [x] 3b.4 `act.Held` is deleted with its one edge. Nothing produced it: `ClaimLeaf` and
      `ClaimContainer` answer `(true, nil)` or an error, so every "held" was a fault wearing the word
      for a rule.
- [x] 3b.5 `core.BaseBranch` says there is no reference branch to work from when the checkout has no
      branch. The unconfigured fallback is the design — the branch checked out IS the reference — so
      the error names the missing branch and nothing about `reference:`.

## 3c. The hub tells, and never sends the agent to ask

- [x] 3c.1 Twenty agent-facing replies drop the bare `sindri` pointer: they state what is refused,
      what to do instead where there is something, and nothing where there is not. Across
      `prompts/{prompts,prompts_feature,prompts_mail,injected,submitreplies}.go`,
      `flow/task/read_act.go`, `flow/pr/verdict_act.go`, `api/agents/registry/registry.go` and
      `hub/commands.go`.
- [x] 3c.2 `mail.MsgMailWaiting` keeps its pointer — the only case where the AGENT chooses the
      timing. `ReplyNoMail` loses its trailing one: an empty mailbox settles nothing to time.
- [x] 3c.3 `ReplyResolveDirty` gains a `resolving` case. It was falling through to the pointer,
      which is why the default branch had one at all — the phase has a real next step, `sindri
      resolve` after the markers are fixed.
- [x] 3c.4 `TestNothingTellsAnAgentToAskForItsDirective` walks `internal/` and fails on any bare
      `run \`sindri\`` outside the mail announcement. Proved by restoring the pointer on
      `ReplyNothingToRevoke` and watching it name the file and line.

## 3d. No state holds an agent that has nothing to be there for

- [x] 3d.1 `cond.HoldsNothing` is read by every PARKING state that means an agent at work —
      `working`, `gating`, `between-subtasks`, `feature-gated`, `feature-done`, `stalled` — each
      leading to `idle`, under the escalation exit alone.
- [x] 3d.2 The rule is about the state's SHAPE, not a list: a state whose action lands it somewhere
      always leaves, so only one nothing but a condition moves can hold anybody. `parks()` derives
      that from the map.
- [x] 3d.3 `TestNoWorkStateHoldsAnEmptyHandedAgent` holds it, with `holdsNothingIsFine` naming each
      exemption and why — `idle`, the four states a pull request anchors, and the three lifecycle
      states every role shares. `TestEveryExemptionNamesAParkingState` keeps that list honest.
- [x] 3d.4 Proved by removing the `working` edge and watching the guard name it.
- [x] 3d.5 `TestStalledForIsWhatTheBoardAndTheNudgeShare`'s fixture now states what each placement
      holds. It placed a "working" agent with an empty row, which the map no longer allows to stand.

## 4. Guards

- [x] 4.1 `TestTheClaimChainOnlyRunsForward`: every edge inside the chain leads to a later step.
      Proved by restoring `retiering --done--> assigning` and watching it fail.
- [x] 4.2 `TestPreparationIsEnteredByAClaimAlone`: no resting state leads into preparation. Proved by
      restoring `between-subtasks --subtask-ready--> retiering`.
- [x] 4.3 `TestEveryChainStepEndsAtTheWork`: no step after a selection falls back to `idle`.
- [x] 4.4 `TestSetModelSwitchesASessionTheRecordAlreadyClaims`: the four-day bug in one call.
- [x] 4.5 `TestTheSituationJoinsTheRecordWithTheTranscript` and `TestTierIsReadsAModelsFamilyNotItsExactID`.

## 5. Verify

- [x] 5.1 `go test ./internal/... ./cmd/...` passes.
- [x] 5.2 `brokkr lint` passes, all ten linters.
- [x] 5.3 `openspec validate --all` passes for this change.
