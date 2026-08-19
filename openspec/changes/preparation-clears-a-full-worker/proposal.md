# A full worker is cleared and handed its next task, not retired until a human acts

## Why

`retire-a-full-worker` gave the hub a hard stop: past `ContextFullFraction` (85%), `claimNext`
skipped a worker outright and told it directly (`DirFull`) — "retired from assignment until a human
clears you". That was honest when it was written: compaction had to be armed by a human
(`sindri agent clear-context <name>`), so a full agent genuinely could not be given work, and saying
so was the only correct answer.

That premise is gone. Every assignment already runs a preparation step first — `prepareAssignment`
claims the task, then fires a model switch or a compaction before the agent is told anything
(`DirPreparing`). The fullness gate sat in front of that step rather than inside it: `claimNext`
checked `contextFull` and refused *before* ever reaching the preparation that would have fixed it.
An agent asking for work is, by definition, at a leaf boundary — the exact condition a clear needs —
so refusing it there and waiting for a human to notice and run the same clear by hand added a stall
with no remaining reason to exist. It also produced a genuine deadlock: `austri`, the worker whose
979k-token session tripped this exact gate, could not be handed the task describing the bug, because
being handed it was the gated action.

## What Changes

- `claimNext` (and the two `AgentDirective` paths that used to short-circuit ahead of it,
  `waitForNextTask` and `CmdNext`) no longer refuse a worker for being full. The claim proceeds
  exactly as it would for any worker.
- `prepareAssignment` gains a second preparation: past `ContextFullFraction`, it fires a clear
  (`FireClear`) instead of a compaction — a session that far gone is not worth summarizing. Model
  switch still takes priority (an unrelated concern); a merely-`compactDue` fill still compacts.
  Either way the caller answers `DirPreparing`, and the claimed directive is re-served once the
  clear lands, the same as it already was for compaction.
- `DirFull` is deleted — it has no callers once nothing refuses on fullness.
- Fullness stops meaning "wound down, needs a human": `parkedByTheHub` no longer exempts a full
  worker from the stall nudge (nothing parks it anymore — it clears itself on its next ask), and
  `agentBlocked` no longer reports fullness as a reason nothing is assigned.
- The board's `full` status word goes with it — `overlayFullness`, `api.StatusFull`, and their
  reader in `AgentNeedsUser` are removed, since fullness no longer waits on the user. The raw
  numbers (`ContextTokens`/`ContextWindow`) stay on `AgentView` exactly as before; only the status
  word that meant "someone must act on this" is gone, because nobody has to anymore.
- The manual remedy is untouched: `sindri agent clear-context <name>` still arms and fires a clear
  by hand at a leaf boundary, for a human who wants to clear an agent that isn't full.

## Impact

- Supersedes the assignment-gating requirement added by `retire-a-full-worker` (unarchived) and the
  board-status requirement added by `fullness-explains-only-an-idle-agent` (unarchived) — both
  described exactly the behaviour this proposal removes. Neither should be archived into the base
  spec as written; this proposal's deltas describe where fullness ends up instead.
- Source: `internal/hub/workflow/{fullness,task,explain,stall}.go`, `internal/hub/workflow/prompts.go`
  (`DirFull` removed), `internal/hub/state.go` (`overlayFullness` removed), `internal/api/{board,attention}.go`,
  `internal/ui/tui/theme.go`, `internal/ui/cli/agent.go`, `internal/hub/commands/sections.go`, `README.md`.
- No change to `FireClear`, `ClearArmed`, or the manual `clear-context` command — this reuses them
  as the preparation step, exactly as compaction already did.
