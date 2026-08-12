# A working agent is working

## Why

`internal/hub/state.go` applied `"full"` to an agent's status unconditionally, after every other
word had been decided, so it won over all of them. A worker mid-task read `full`. That says
nothing true about it: it IS working, and the word replaced the one fact the status column exists
to carry.

Fullness is not an activity. It is a REASON — the reason an idle agent is not being handed the
next task. It informs only where there is something to explain: the agent holds nothing and is
being passed over.

The wrong word invites the wrong action. A human scanning the column sees `full` on an agent
mid-task and reaches to clear its context, which the hub refuses at a leaf boundary
(`agent/clearcontext.go`) — so they discover the board was lying only after trying to act on it.

## What changes

- `"full"` applies only where the status would otherwise be `idle` AND the agent holds nothing:
  no task, no feature, no PR. Everywhere else the status keeps what it was.
- `working`, `blocked`, `submitted` and `down` are left alone.
- `stalled` is left alone too. It sits directly above fullness and has the same shape, but a
  stalled agent HOLDS work, and where an agent is both full and stalled, stalled is the more
  actionable word.
- Held work is checked directly rather than inferred from the word. A quiet runtime probe reads a
  task-holder as `idle` before it has been still long enough to say `stalled`, and that agent is
  not idle in the sense fullness explains.
- The decision moves into `overlayFullness`, a pure function beside `overlayRuntime`, so the rule
  is stated once and can be tested without standing up a hub.

## Impact

- Specs: `view-workers` gains a requirement for what the word means. Context fullness was
  unspecified — neither the retirement gate nor the status word appears in any spec today.
- Code: `internal/hub/state.go`, and the `AgentView` doc in `internal/api/board.go`.
- The gate is untouched and stays as it is: `workflow.contextFull` gates `claimNext` only, never
  the directive for work already held, as `TestFullnessGatesNewWorkOnly` pins. This change is
  about the board, not about when an agent is retired.
- Nothing is hidden by this. An agent's fill is on the board as `ContextTokens`/`ContextWindow`
  whatever its status reads, so a full agent mid-task is still visible as full to anyone who
  wants the number.
