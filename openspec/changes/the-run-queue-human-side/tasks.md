# Tasks

## 1. The verb, in both front-ends

- [x] 1.1 `ScheduleUserRun` on the engine, `POST /run/new`, one client method, and both front-ends
      calling it — the same single slot agent runs join.
- [x] 1.2 `sindri run new [--agent] [--priority] [--timeout] <command…>`.
- [x] 1.3 `N` on the Runs tab, queueing against the repo's checkout, with the target in the prompt.

## 2. The target, decided

- [x] 2.1 An agent's workspace, named; or the repo's own checkout. Explicit in the request, never
      taken from the working directory.
- [x] 2.2 The repo's checkout is ALLOWED, and the reason is on the function: the executor mounts a
      copy, so nothing a command writes reaches the tree the user is editing.
- [x] 2.3 A PR's review checkout is OUT, and the proposal says why — `.worktrees/review` is one
      shared slot, and `pr verify` already answers that question synchronously.
- [x] 2.4 `ExecuteRun` materialises `r.Workspace` rather than re-reading the agent. Required for a
      run with no agent, and it fixes a snapshot the executor was storing and ignoring.

## 3. Priority

- [x] 3.1 A user's run leads the queue, gate runs included — somebody is waiting on it, and what
      that costs a parked agent is bounded by the cap.
- [x] 3.2 Still reprioritisable by hand: the origin decides where it starts, not that it is stuck.

## 4. Whose run it is

- [x] 4.1 `Run.Agent` carries the user sentinel; `api.RunFromUser` is the one predicate the queue,
      the executor and both front-ends read.
- [x] 4.2 Rendered as "you" in both front-ends, and the target named beside it.
- [x] 4.3 Nothing agent-shaped applies: no staleness check, no injected result.

## 5. Pin it

- [x] 5.1 The default target, the named one, and the refusal for an agent that does not exist.
- [x] 5.2 The queue order — user, then gate, then ordinary — and that it stays reprioritisable.
- [x] 5.3 A user run is never stale, while an agent run whose agent is gone still is.
- [x] 5.4 A user run injects nothing; an agent's still gets its summary.
