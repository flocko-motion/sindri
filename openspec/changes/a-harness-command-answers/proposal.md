# A harness command blocks and answers

## Why

There are two jobs in this hub and one seam between them, and the seam is in the wrong place.

The **harness** is a sandbox with an agent inside: a container, a tmux session, a Claude Code process
that reads typed input and answers slash commands. The **orchestrator** decides what should happen
next: who holds what, when work is handed over, when a session should start fresh.

Today the orchestrator knows how the harness works. `workflow` — the ruleset — carries `/clear`
timing, what a queued `/clear` does to the queue behind it, model-switch dialogs, compaction,
interrupts and leaf boundaries. The clear alone reaches 103 references across 28 files, of which
`workflow/task.go`, `workflow/fullness.go`, `workflow/review.go`, `workflow/reviewhealth.go`,
`workflow/feature.go`, `workflow/engine.go` and `workflow/prompts.go` are ruleset, not harness.

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

## Impact

- Specs: `agent-runtime` gains the blocking-command contract; `05-workflow` loses the clear-pending
  directive and the assignment window.
- Code: `internal/hub/agent` (`clearcontext.go`, `compact.go`, `model.go`, `boundary.go`),
  `internal/hub/workflow` (`engine.go`, `task.go`, `fullness.go`, `review.go`, `prompts.go`).
- Behaviour the user sees is unchanged, with one exception that is the point of the change: a clear
  that does not take effect now surfaces as a failure rather than an agent that has gone quiet.

## Where this sits

First of three, and deliberately the one that is pure subtraction. `one-surface-for-what-is-allowed`
gathers the scattered rules into one situation struct, and `the-harness-reports-observations`
finishes the seam by making the evidence those rules read cross it whole. This goes first because
the assignment window it deletes would otherwise be built INTO that situation struct and have to be
unpicked afterwards.
