# Tasks

## 1. Queue the gate instead of running it inline

- [x] 1.1 `enqueueGate` queues a submit/contribute gate through the same run queue an
      agent-requested run uses, tagged with its kind (`submit`/`contribute`) and the agent's
      free-text summary.
- [x] 1.2 `executeGateRun` runs it via the existing host-subprocess gate against the agent's live
      worktree — no container, no materialization, the same check an inline gate always ran.
- [x] 1.3 A gate run outranks an exploratory one in `queuePositions` unconditionally, not just by
      priority code.

## 2. The submit contract: queue and return, land later

- [x] 2.1 `CmdSubmit`/`CmdContribute` keep every synchronous precondition check (phase, open
      subtasks, gated children, behind-base refusal) and then queue the gate and reply at once
      with its position — no PR yet.
- [x] 2.2 `completeGate` lands the continuation once the gate settles: `landGate` creates the PR
      only on "passed"; `rejectGate` puts the agent back to `working` with the violations, no PR.
- [x] 2.3 `stallGate` handles a gate that never reached a verdict (timeout, or cancelled by a hub
      restart) — back to `working`, told to try again, never as a lint failure.

## 3. The `gating` phase

- [x] 3.1 Added alongside `submitted`/`resolving`; every place that gated on those treats `gating`
      the same way: `AgentDirective`'s three switches, `Stalled()`, `landingBlocked`,
      `referenceMoved`'s under-review check.
- [x] 3.2 `ReplyNotWorking`/`ReplyResolveDirty` get honest `gating`-specific wording rather than
      falling through to the generic phase message.
- [x] 3.3 NOT extended to `revoke` — withdrawing a queued/running gate needs its own cancellation
      path, out of this proposal's scope.

## 4. Pin it

- [x] 4.1 A passed gate creates the PR with the same commit message an inline gate would have
      produced; a failed one creates none and returns the agent to `working`.
- [x] 4.2 A timed-out or restart-orphaned gate is reported as incomplete, distinct from a
      violation, and does not create a PR.
- [x] 4.3 A gate run always outranks a queued exploratory run regardless of priority or arrival
      order.
- [x] 4.4 Every existing submit/contribute test drives the gate through the queue
      (`runQueuedGate`) rather than expecting an inline result.
