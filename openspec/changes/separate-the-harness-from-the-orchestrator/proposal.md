# Separate the harness from the orchestrator: a harness command blocks and answers

## Why

There are two jobs in this hub and one seam between them, and the seam is in the wrong place.

The **harness** is a sandbox with an agent inside: a container, a tmux session, a Claude Code process
that reads typed input and answers slash commands. The **orchestrator** decides what should happen
next: who holds what, when work is handed over, when a session should start fresh.

Today the orchestrator knows how the harness works. `workflow` — the ruleset — carries `/clear`
timing, what a queued `/clear` does to the queue behind it, model-switch dialogs, compaction,
interrupts, leaf boundaries, and "would a message sent now be seen". `Deps` is 26 methods and over
half of them are session mechanics. The clear alone reaches 103 references across 28 files, of which
`workflow/task.go`, `workflow/fullness.go`, `workflow/review.go`, `workflow/reviewhealth.go`,
`workflow/feature.go`, `workflow/explain.go`, `workflow/engine.go` and `workflow/prompts.go` are
ruleset, not harness.

**The root cause is that harness commands do not answer.** `FireClear` types `/clear`, returns, and
finishes the job in a detached goroutine: it waits for the context reading to fall, then injects a
follow-up. `SetModel` and `Compact` have the same shape. Because none of them answers, each takes a
`next string` — the message to queue behind the operation — and the orchestrator has to carry the
sequencing:

- a `next` argument on three commands, threaded from five call sites, usually `MsgKickoff`
- `DirClearPending`, a directive whose only job is answering an agent that asks during the window
- `clearArmed` / `fireClearIfArmed`, checked along several directive paths
- `BeginAssignment` / `EndAssignment`, a window that stops `AtLeafBoundary` refusing the very step
  the gate is running
- `WakeRefusal`, consulted by `hub.Deliver` — so delivery asks the ruleset a question, and callers
  that must bypass it need `.Regardless()`

The dependency also runs both ways. `workflow` reaches the harness through `deps.Deliver`, and
`hub.Deliver` calls back into `workflow.WakeRefusal`. That cycle exists because "may this land?" and
"what should be said?" are answered in the same place.

And it is not merely untidy: because `FireClear` returns before the work is done, **a failure after
it returns has nobody to report to**. `awaitCleared` gives up after five minutes, writes
`clear-unconfirmed` to the agent's log, and returns. The caller has long since replied. That is how
an agent came to sit idle after a clear with no instruction — the fix at the time was to make the
goroutine wait for evidence instead of a timer, which made the symptom rarer without giving the
failure anywhere to go.

## What Changes

**A harness command blocks and answers.** `Clear`, `Compact` and `SetModel` become synchronous: they
take a context, do the whole operation, and return success, timeout, or a stated error. They no
longer take `next`. The caller, having an answer, sends the next instruction through the ordinary
delivery path — which is also the path that already reports its own failures.

This is what removes the machinery rather than moving it:

- `next` disappears from three signatures and five call sites.
- The kickoff goroutine, `kickoffWG` and the test hook that joins it disappear: there is nothing
  detached to wait for.
- `DirClearPending` disappears. There is no window to answer, because the call that fires the clear
  does not return until it is over.
- `BeginAssignment`/`EndAssignment` disappear. They exist to keep `AtLeafBoundary` from refusing a
  step inside an assignment that is already in flight; with the operation inside one call, the
  caller knows it is mid-assignment without a flag in the store.
- A clear that fails is a returned error at the point of decision, so an agent that cannot be
  cleared is a thing the orchestrator can act on — escalate it, retry it, leave it alone — instead
  of a log line nobody reads.

**The harness reports observations; the orchestrator draws conclusions.** These are different kinds
of statement and today they are the same sentence. Whether a process is up, whether the pane is at a
prompt, how long the screen has been unchanged, how large the transcript is — those are facts about
a box, and only the harness can see them. Whether an agent is *stalled*, *idle*, *blocked* or *full*
are judgements about work, true only in the light of what the agent holds.

Today the workflow reads raw evidence and the harness ships conclusions. `workflow/stall.go`
switches on the literal strings `"blocked"`, `"signed-out"` and `"api-error"` off the pane; `hub`
folds the same word into a board status in `overlayRuntime`. Meanwhile the whole standing
observation — up, clients, the runtime word, the pane digest, how long it has been still, whether a
tool is in flight, the context fill — reaches the workflow through three booleans (`AgentAlive`,
`AgentUp`, `AgentIdle`). Everything else is either re-derived or passed as an untyped string.

So the harness SHALL report one observation value carrying the evidence and when it was taken, and
the orchestrator SHALL be where a word like "stalled" is defined. The naming rule that follows: the
harness may say *what it saw*, never *what it means*. `Idle` is the case to watch — "at an empty
prompt" is an observation, "has nothing to do" is a conclusion, and one name has been carrying both.

The reason this is worth doing is debugging. With the observation a value in its own right, what the
hub SAW can be shown beside what it CONCLUDED, and a wrong answer is one question — was the evidence
wrong, or the rule?

**The seam is named and the interface is split.** `workflow.Deps` becomes two:

- `Harness` — everything about driving one agent in its box: `Observe`, `Say`, `Clear`, `Compact`,
  `SetModel`, `Interrupt`, `Start`, `Container`. It knows tmux, pods and slash commands, and nothing
  about tasks, PRs or reviews. `Observe` replaces `AgentAlive`/`AgentUp`/`AgentIdle` with the one
  value they were each cutting a boolean out of.
- `Deps` — what is left: the project's facts, the board, the task thread, the mailbox.

The split is the check on every future change: a method that names a task belongs on the second, a
method that names a keystroke belongs on the first, and a method that names both is the next thing
to pull apart.

**Delivery stops asking the ruleset for permission.** `WakeRefusal` moves to the sender's side of
the call: `hub.Deliver` carries out a classified delivery and reports what happened, and the decision
about whether waking this agent is worth it is made by whoever composes the message. `.Regardless()`
goes with it — an escape hatch is only needed while the gate is somewhere the caller cannot see.

## Impact

- Specs: `agent-runtime` gains the harness contract (a command blocks and answers); `05-workflow`
  loses the clear-pending directive and the assignment window.
- Code: `internal/hub/agent` (`clearcontext.go`, `compact.go`, `model.go`, `boundary.go`),
  `internal/hub/workflow` (`engine.go`, `task.go`, `fullness.go`, `review.go`, `reviewhealth.go`,
  `feature.go`, `explain.go`, `prompts.go`), `internal/hub/deliver.go`, `internal/hub/wiring.go`.
- Behaviour the user sees is unchanged, with one exception that is the point of the change: a clear
  that does not take effect now surfaces as a failure rather than an agent that has gone quiet.

## Staging

The seam is worth having on its own, but the blocking commands are what make it hold, so they come
first and each is provable alone:

1. `Clear` blocks and answers; `next` and `DirClearPending` go. This is the user's own example and
   the largest single reduction.
2. `Compact` and `SetModel` follow the same shape.
3. `Observe` replaces the three booleans, and the words built on it move to the orchestrator.
4. The `Harness`/`Deps` split, which by then is mostly a move.
5. `WakeRefusal` moves to the sender; `.Regardless()` goes.
