# Tasks

## 1. Clear blocks and answers

- [ ] 1.1 `agent.Service.Clear(ctx, project, name) error` replaces `FireClear`: fire at a leaf
      boundary, then wait for the context reading to fall (the existing `awaitCleared` evidence
      check, promoted from a goroutine to the call itself). Return nil on success, a timeout error
      naming the agent and the wait, or the underlying failure. No `next`, no `interrupt` — an
      interrupt is its own command a caller issues first if it means to.
- [ ] 1.2 Delete the kickoff goroutine, `kickoffWG` and the test hook that joins it
      (`waitForKickoff`); nothing is detached anymore.
- [ ] 1.3 Each of the five `FireClear` call sites becomes clear-then-deliver: `workflow/engine.go`
      (`fireClearIfArmed`), `workflow/review.go`, `workflow/fullness.go`, and the two in
      `agent/clearcontext.go` (`ClearContextNow`, `FireArmedClears`). Each now has an error to
      handle — say what each does with it rather than dropping it.
- [ ] 1.4 Delete `DirClearPending` and `mailDeferred`'s branch for it.
- [ ] 1.5 Delete `BeginAssignment`/`EndAssignment` and the store field behind them; the caller is
      inside the assignment by construction.
- [ ] 1.6 A test that a clear which never lands returns a failure, and that the caller's agent is
      not left silently idle — the failure mode that motivated this.

## 2. Compact and SetModel follow

- [ ] 2.1 `Compact(ctx, project, name) error` — same shape, `next` gone.
- [ ] 2.2 `SetModel(ctx, project, name, model) error` — same shape, `next` gone. It already
      sequences a clear then the switch internally; with both blocking, that sequence is two calls
      in a row rather than nested waits.
- [ ] 2.3 `prepareAssignment` (`workflow/fullness.go`) becomes straight-line: switch or clear, check
      the error, then deliver the claimed directive. `DirPreparing` survives only if something still
      genuinely lands behind this call — if nothing does, delete it too and say so here.

## 3. Observations cross the seam whole

- [ ] 3.0 Lift the watchdog's `liveness` into a named observation on the harness — up, clients, the
      session's own runtime word, pane digest, still-since, tool-since, fill, and the time it was
      taken — and give the harness one `Observe(project, name)` returning it. `AgentAlive`,
      `AgentUp` and `AgentIdle` go: each was a boolean cut out of this value.
- [ ] 3.1 Move the interpretations to the orchestrator, by name. `workflow/stall.go` stops switching
      on the literal `"blocked"`/`"signed-out"`/`"api-error"` and reads named fields; `hub`'s
      `overlayRuntime` folds an observation rather than a string. Where "idle" means "at a prompt"
      it stays an observation; where it means "has nothing to do" it becomes an orchestrator word
      with its own name — the one place the two senses are currently the same identifier.
- [ ] 3.2 Show it: the agent detail surfaces the observation as taken, beside the status word
      derived from it, so a wrong answer is one question — bad evidence, or a bad rule. This is the
      point of the split, so it is not optional.

## 4. The interfaces

- [ ] 4.1 Split `workflow.Deps` into `Harness` (Observe, Say, Clear, Compact, SetModel, Interrupt,
      Start, Container) and `Deps` (the rest). Engine holds both.
- [ ] 4.2 Move `CompactionThreshold`, `ModelForTier` and `ModelMatches` deliberately: each names a
      model rather than a session, so decide whether it is harness or policy and record which and
      why — this is the case the split exists to make legible.
- [ ] 4.3 An arch test that `Harness` names no task, PR, review or verdict, and that its observation
      type carries no judgement word, so the seam is enforced rather than described (`internal/arch`
      already holds this kind of guard).

## 5. Delivery stops asking the ruleset

- [ ] 5.1 Move the `WakeRefusal` check out of `hub.Deliver` to the callers that compose messages, so
      delivery carries out what it is given and reports the outcome.
- [ ] 5.2 Delete `Delivery.Unconditional` and `.Regardless()`; with the gate on the sender's side
      there is nothing to bypass. Every current `.Regardless()` call site is a message that IS the
      exit from a refusing state — check each still sends, and that a test covers it.
- [ ] 5.3 Confirm the cycle is gone: `hub` may call `workflow`, or `workflow` may call `hub` through
      its interfaces, but the delivery path no longer does both.
