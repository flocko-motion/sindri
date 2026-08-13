# Clearing a context: confirm, then arm

## Why

The UI confirms and the hub then refuses. `tab_agents.go` asks "clear <name> context? (its session
starts empty)", and `agent/clearcontext.go` answers "still holds sd-xxxx — clearing only applies at
a leaf boundary, once it's idle". So the one destructive action a user is asked to confirm is also
the one that most often fails after they confirm it, and the remedy for a full agent is unavailable
in exactly the state that fills it: mid-task.

The refusal is right. What is wrong is that it is a dead end rather than a wait.

## What changes

- Asking for a clear ARMS one. The user decides whether; the agent's state decides when. An agent at
  a leaf boundary is cleared at once; one holding work keeps the arming until it reaches one —
  finishing its task, checkpointing within a feature, or delivering its verdict.
- A boundary is holding no leaf task and owing no verdict. A held feature is not such a thing: the
  clear fires between subtasks, and the next subtask's directive names the feature afresh. Planners
  and coauthors hold neither, so they are always at one.
- An armed agent is handed no new leaf work — through `claimNext`, through the feature's own
  advance, and through the reviewer assignment — and is told why (`DirClearPending`). Without that
  gate the clear would land into work served in the meantime, meeting the boundary rule it was
  waiting for.
- The arming outranks the fullness notice. `DirFull` says "retired from assignment until a human
  clears you"; once one has, repeating it would park the arming behind a state that never advances.
- The arming is durable (an `agents.clear_armed` column) — the hub may restart between the arming
  and the boundary, and an arming that evaporated would leave the user believing it was set.
- It fires from the hub's own tick, never from the agent's request: the clear interrupts the
  session, and an agent that has just asked for work is mid-turn, holding the very command that
  would be cut off.
- The modal confirms and states when; it does not offer a choice of when. Pressing the key again
  disarms with no modal — cancelling a destructive action is not itself destructive. Both are
  visible: a marker on the row, a line in the detail, and the same marker in `sindri agent list`.

## Impact

- Specs: `agent-runtime`'s clear requirement becomes an arming (it is a MODIFIED delta over the one
  `retire-a-full-worker` introduces, which describes the refusal this replaces); `view-workers`
  gains the visibility and the toggle.
- Code: `internal/hub/store` (the column), `internal/hub/agent/clearcontext.go` (arm, boundary,
  fire), `internal/hub/workflow` (the gate, the directive, the checkpoint), `internal/hub/refwatch.go`
  (the tick), `internal/api` (the flag and the "when" rule), `internal/client`, and both front-ends.
- `client.ClearContext` becomes `client.SetClearArmed(name, armed)` — one method for a toggle, so
  the CLI's `--cancel` and the TUI's second press are the same operation.
- No path clears an agent mid-task: the fire re-checks the boundary itself, so the sweep and the
  arming cannot disagree even if the agent picked something up between them.
