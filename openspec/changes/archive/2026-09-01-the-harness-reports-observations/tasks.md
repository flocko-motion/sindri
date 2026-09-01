# Tasks

## 1. One observation crosses the seam

- [x] 1.1 Lift the watchdog's `liveness` into a named observation on the harness — up, clients, the
      session's own runtime word, pane digest, still-since, tool-since, fill, and the time it was
      taken — and give the harness one `Observe(project, name)` returning it. It lives in its own
      package, `internal/hub/observe`, so the workflow, the agent service and the situation can all
      read it without any of them importing another.
- [x] 1.2 Delete `AgentAlive`, `AgentUp` and `AgentIdle`: each was a boolean cut out of that value.
      `AgentAlive` PROBES where the other two read the standing reading, so the observation has to
      carry its own freshness for a caller that needs the answer as of now — that distinction is
      real and must survive the merge, not be flattened away. It survives as two harness methods:
      `Observe` hands over the standing look, `Probe` takes a fresh one, and both stamp `TakenAt`.
- [x] 1.3 An arch test that the observation type carries no judgement word (`stalled`, `blocked`,
      `full`, `needs`), so the seam is enforced rather than described
      (`internal/arch`: TestTheObservationCarriesNoJudgement — `idle` is on its list too, for 2.3).

## 2. The words move to the orchestrator

- [x] 2.1 `workflow/stall.go` stops switching on the literal `"blocked"`/`"signed-out"`/
      `"api-error"` and reads named fields off the observation.
- [x] 2.2 `hub`'s `overlayRuntime` folds an observation rather than a string. It folds it in the
      SURFACE rather than in `hub`: with the lifecycle intent now on the observation, the whole
      status word is derivable from evidence plus held work, so keeping half of it in `hub` would
      have left two homes for one word. `agent.Service` keeps only the WRITE that a fold cannot own
      — retiring an intent reality has caught up with (`SettleIntent`).
- [x] 2.3 Split the two senses of "idle". Where it means "at a prompt" it stays an observation
      (`observe.Observation.AtPrompt`); where it means "has nothing to do" it is the orchestrator's
      (`situation.Situation.HoldsNothing`). Neither name can now be read as the other.
- [x] 2.4 Show it: the agent detail surfaces the observation as taken, beside the status word
      derived from it, so a wrong answer is one question — bad evidence, or a bad rule. Both
      front-ends carry it (`observed:` in `sindri agent info` and in the TUI's agent detail), off
      two decided fields on the board rather than a rule either of them applies.

## 3. The interfaces

- [x] 3.1 Split `workflow.Deps` into `Harness` (Observe, Say, Clear, Compact, SetModel, Interrupt,
      Start, Container) and `Deps` (the rest). Engine holds both. `Probe` joins the harness for 1.2,
      and `Deliver` becomes `Say`.
- [x] 3.2 Place `CompactionThreshold`, `ModelForTier` and `ModelMatches` deliberately: each names a
      model rather than a session, so decide whether it is harness or policy and record which and
      why — this is the case the split exists to make legible.

      DECIDED: `ModelMatches` and `CompactionThreshold` are HARNESS. Both are the backend's knowledge
      of its own models — whether two ids name one model, and what fill is worth compacting for a
      given window — and only the thing running the session can answer either. `ModelForTier` is
      DEPS: which model a difficulty tier deserves is the hub's policy, and it would be the same
      question against any backend.
- [x] 3.3 An arch test that `Harness` names no task, PR, review or verdict
      (`internal/arch`: TestTheHarnessNamesNoWork, over the method and parameter names).

## 4. Delivery stops asking the ruleset

- [x] 4.1 Move the `WakeRefusal` check out of `hub.Deliver` to the callers that compose messages, so
      delivery carries out what it is given and reports the outcome.
- [x] 4.2 Delete `Delivery.Unconditional` and `.Regardless()`; with the gate on the sender's side
      there is nothing to bypass. Every current `.Regardless()` call site is a message that IS the
      exit from a refusing state — check each still sends, and that a test covers it.

      All twelve still send, and none needed a gate of its own: the nudge sweeps and the stall nudge
      already ask the surface before composing (that landed with `one-surface-for-what-is-allowed`),
      and the rest — the post-clear kickoffs, the mail announcement, the escalation notice, the
      gate-missing escalation, the launch greeting — are the exits themselves. ONE sender did rely on
      the old gate and now makes the judgement itself: `Hub.SetRetired`'s un-retire notice, which
      must not wake an agent still stuck on an escalation. It records the refusal too, since the
      delivery path no longer knows one happened (-> TestUnretiringAnEscalatedAgentStillWaitsOnTheEscalation).
- [x] 4.3 Confirm the cycle is gone: `hub` may call `workflow`, or `workflow` may call `hub` through
      its interfaces, but the delivery path no longer does both. `hub.Deliver` names no workflow rule;
      `WakeRefusal` is now asked only inside `workflow` and by the one sender above, through the
      situation rather than through the engine.

## 5. Hold the seam (added on review)

The epic's own finish line is "an arch test holds the seam so a sixth violation cannot appear
quietly". 1.3 and 3.3 guard two invariants worth having, but neither is one of the pair the epic
enumerates as the state to preserve — and both of those were true and held by nothing.

- [x] 5.1 The orchestrator cannot reach the box: `internal/hub/workflow` and `internal/hub/situation`
      must not depend on tmux or the coding-tool adapter, walked over the REAL import graph
      (`go list -deps`) so an indirect reach fails too — the shape `internal/ui/importguard_test.go`
      already uses. Proved to fail on a direct import AND on one arriving through an intermediate.
- [x] 5.2 The delivery cycle stays gone: a walk out from `hub.Deliver` fails on any reach into the
      workflow engine. Proved to fail by restoring the `WakeRefusal` check it was written for.
- [x] 5.3 `internal/container` is NOT forbidden outright, because the run queue builds and executes
      in disposable pods that belong to no agent. The import graph cannot tell those from a reach
      into an agent's session, so the exception is pinned to the file that owns it (`execrun.go`, in
      `runQueuePods`) rather than to the package — and a workflow file naming the runtime tomorrow
      has to argue for itself there.

## 6. The state is a type (second review pass)

The word was still crossing the seam as a `string`, which is what let it leak into five places. A
named string type would not have closed it — a bare literal compares equal to one — so the state is
an INTEGER, and the words live in one table.

- [x] 6.1 `observe.State` (uint8) with Working, AtPrompt, AwaitingHuman, SignedOut, TurnCutOff, and
      Unknown as the zero value. `ParseState` reads the tool's word ONCE, at the watchdog's boundary;
      `String` renders it back for the board and the front-end projections. Nothing between the two
      holds a word. In hub/observe rather than adapter/agent, because the orchestrator reads a state
      through this package and importing that adapter is what the seam forbids it.
- [x] 6.2 `Observation.Runtime string` becomes `Observation.State State`, and `liveness.runtime`
      likewise — so `o.State == "api-error"` is a compile error rather than a review catch.
      `RuntimeSince` becomes `StateSince`, since it times that account changing.
- [x] 6.3 Every reader converted, not just the one named: workflow/stall.go (which now takes the
      OBSERVATION rather than a word), situation/surface.go's overlayRuntime, hub/stallwatch.go,
      hub/credwatch.go and hub/watchdog.go. Derived in both directions — every literal comparison
      against a runtime word across the tree, and every remaining occurrence of the five words. What
      is left is the workflow's own PHASE vocabulary, which shares two of the words and is a
      different thing, plus the two front-end projections that cannot ask (adapter/herdr.State and
      the TUI's status colouring), which read the word off the board.
- [x] 6.4 `TestNoPaneWordIsMatchedInTheHub` covers the whole hub, not only the orchestrator: three of
      the five leaks were on the observer's side. It reads the vocabulary out of state.go's table, so
      a sixth word is guarded the day it is added, and is proved to fail on both the comparison and
      the switch form. The type is the real defence now; this is the backstop for `AgentView.Runtime`,
      which still crosses the wire as a string.
- [x] 6.5 observe.go's header claim was false and is now bounded to what holds: nowhere in the HUB is
      a runtime word compared to a literal, and the two front-end projections that do are named with
      the reason they cannot ask.
