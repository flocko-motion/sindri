# Tasks

## 1. One observation crosses the seam

- [ ] 1.1 Lift the watchdog's `liveness` into a named observation on the harness — up, clients, the
      session's own runtime word, pane digest, still-since, tool-since, fill, and the time it was
      taken — and give the harness one `Observe(project, name)` returning it.
- [ ] 1.2 Delete `AgentAlive`, `AgentUp` and `AgentIdle`: each was a boolean cut out of that value.
      `AgentAlive` PROBES where the other two read the standing reading, so the observation has to
      carry its own freshness for a caller that needs the answer as of now — that distinction is
      real and must survive the merge, not be flattened away.
- [ ] 1.3 An arch test that the observation type carries no judgement word (`stalled`, `blocked`,
      `full`, `needs`), so the seam is enforced rather than described.

## 2. The words move to the orchestrator

- [ ] 2.1 `workflow/stall.go` stops switching on the literal `"blocked"`/`"signed-out"`/
      `"api-error"` and reads named fields off the observation.
- [ ] 2.2 `hub`'s `overlayRuntime` folds an observation rather than a string.
- [ ] 2.3 Split the two senses of "idle". Where it means "at a prompt" it stays an observation;
      where it means "has nothing to do" it becomes an orchestrator word with its own name. This is
      the one place the two senses are currently the same identifier, so name both here.
- [ ] 2.4 Show it: the agent detail surfaces the observation as taken, beside the status word
      derived from it, so a wrong answer is one question — bad evidence, or a bad rule. This is the
      point of the split, so it is not optional.

## 3. The interfaces

- [ ] 3.1 Split `workflow.Deps` into `Harness` (Observe, Say, Clear, Compact, SetModel, Interrupt,
      Start, Container) and `Deps` (the rest). Engine holds both.
- [ ] 3.2 Place `CompactionThreshold`, `ModelForTier` and `ModelMatches` deliberately: each names a
      model rather than a session, so decide whether it is harness or policy and record which and
      why — this is the case the split exists to make legible.
- [ ] 3.3 An arch test that `Harness` names no task, PR, review or verdict.

## 4. Delivery stops asking the ruleset

- [ ] 4.1 Move the `WakeRefusal` check out of `hub.Deliver` to the callers that compose messages, so
      delivery carries out what it is given and reports the outcome.
- [ ] 4.2 Delete `Delivery.Unconditional` and `.Regardless()`; with the gate on the sender's side
      there is nothing to bypass. Every current `.Regardless()` call site is a message that IS the
      exit from a refusing state — check each still sends, and that a test covers it.
- [ ] 4.3 Confirm the cycle is gone: `hub` may call `workflow`, or `workflow` may call `hub` through
      its interfaces, but the delivery path no longer does both.
