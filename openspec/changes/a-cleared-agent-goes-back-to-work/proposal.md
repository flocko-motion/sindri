# Clearing a full agent did not put it back to work

## Why

An agent retired for a full context was cleared, kicked, asked the hub for work, and was told
"~900k tokens — past the point of taking on new work. You are retired from assignment until a human
clears you" — the remedy that had just been applied.

It is not a race. `ContextUsage` memoises for `contextTTL` = 15s; the kickoff after a clear fires at
`clearKickoffDelay` = 2s. The ask therefore lands inside the window every time, and the hub answers
from the pre-clear measurement:

    ClearContext -> "/clear" -> sleep 2s -> kickoff
    agent runs `sindri` -> contextFull -> ContextUsage -> MEMOISED 900k -> DirFull

Lengthening the delay would only make it work by luck.

The second fault came from the same session: the parked agent was nudged repeatedly — "nudge stalled
on os-8ea68f — idle for 6m0s / 7m0s / 10m0s". `DirFull` tells it "do not ask again, just wait". It
obeyed, and was chased for obeying.

## What changes

- `ClearContext` drops the agent's cached measurement, before the kickoff. The clear is the one
  moment the hub KNOWS the previous reading is wrong, because it just caused it — so the
  invalidation belongs at the action, not in a shorter TTL. The TTL stays as it is for every other
  reader; it exists so the idle poll does not re-read a transcript per request.
- The board is fixed by the same line: `ContextFull` reads the same memo, so a cleared agent would
  otherwise have shown as full for the rest of the window while holding the work it had just been
  given.
- A parked agent — retired by a human or by its own context — is exempt from the idle nudge.

## A decision my own test forced

I first placed the exemption before the api-error branch, and `TestACutOffTurnIsStillRetried`
failed. That is right: the retry is not a complaint about idling, it asks a turn that stopped
mid-sentence to resume, and being wound down is no reason to leave the last turn broken. The
exemption now covers the idle nudge alone, and the ordering is pinned.

## What is NOT pinned by a test, and why

That `ClearContext` performs the invalidation. The behaviour it enables IS pinned — a stale-full
reading yields `DirFull`, a corrected one yields the waiting task, and the board agrees — and
mutating the exemption, the ordering and the reading each fail their own tests. But removing the
call from `ClearContext` passes, because the tests invalidate directly.

Driving `ClearContext` itself needs a live session: it goes through `container.Running` and a tmux
pane read, neither of which is behind the agent module's `Deps`. Faking both to assert one call is
disproportionate, so this is recorded rather than implied covered. The call is synchronous and the
kickoff is a delayed goroutine, so the ordering it needs holds by construction.

## Impact

- Specs: `hub` gains two requirements — a measurement invalidated by an action, and an agent parked
  by instruction.
- Code: `internal/hub/agent/runtime.go` (`ForgetContext`), `clearcontext.go` (the call),
  `internal/hub/workflow/stall.go` (the exemption).
- The hub's own tests supply an implementation of the coding-agent port rather than the real
  adapter, which is wired at the composition root. That is what the port is for, and it isolates
  the memo, which is the defect.
