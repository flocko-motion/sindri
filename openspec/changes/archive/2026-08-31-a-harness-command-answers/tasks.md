# Tasks

## 1. Clear blocks and answers

- [x] 1.1 `agent.Service.Clear(ctx, project, name) error` replaces `FireClear`: fire at a leaf
      boundary, then wait for the context reading to fall (the existing `awaitCleared` evidence
      check, promoted from a goroutine to the call itself). Return nil on success, a timeout error
      naming the agent and the wait, or the underlying failure. No `next`, no `interrupt` — an
      interrupt is its own command a caller issues first if it means to.
- [x] 1.2 Delete the kickoff goroutine, `kickoffWG` and the test hook that joins it
      (`waitForKickoff`); nothing is detached anymore. `FireClear` itself went with them — it was the
      only holder of the goroutine and had no callers left once 1.3 landed.
- [x] 1.3 Each of the five `FireClear` call sites becomes clear-then-deliver: `workflow/engine.go`
      (`fireClearIfArmed`), `workflow/review.go`, `workflow/fullness.go`, and the two in
      `agent/clearcontext.go` (`ClearContextNow`, `FireArmedClears`). Each now has an error to
      handle — say what each does with it rather than dropping it.
- [x] 1.4 Delete `DirClearPending` and `mailDeferred`'s branch for it.
- [x] 1.5 Delete `BeginAssignment`/`EndAssignment` and the store field behind them; the caller is
      inside the assignment by construction. The window only existed because `Clear` and `Compact`
      checked `AtLeafBoundary` themselves, so the check moved out to the callers that ask it —
      `SetClearArmed` and `FireArmedClears` — and the harness commands now just perform.
- [x] 1.6 A test that a clear which never lands returns a failure, and that the caller's agent is
      not left silently idle — the failure mode that motivated this.

## 2. Compact and SetModel follow

- [x] 2.1 `Compact(ctx, project, name) error` — same shape, `next` gone.
- [x] 2.2 `SetModel(ctx, project, name, model) error` — same shape, `next` gone. It already
      sequences a clear then the switch internally; with both blocking, that sequence is two calls
      in a row rather than nested waits.
- [x] 2.3 `prepareAssignment` (`workflow/fullness.go`) becomes straight-line: switch or clear, check
      the error, then deliver the claimed directive. `DirPreparing` SURVIVES: the directive is pushed
      into the session rather than returned, because the reply to this ask goes to the context the
      reset has just discarded. It absorbs `DirClearPending`'s callers too — `fireClearIfArmed` and
      `reviewDirective` answer it after a clear, which is also what keeps `mailDeferred` holding
      unread mail back from a reply nobody will read.

## 3. Hand over

- [x] 3.1 Say in the task thread what the next change in the tree inherits: which of
      `AtLeafBoundary`'s callers are left, and whether `DirPreparing` survived. Both are inputs
      `one-surface-for-what-is-allowed` builds its situation struct from, and it goes next.
