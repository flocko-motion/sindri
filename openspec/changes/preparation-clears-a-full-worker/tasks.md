# Tasks

## 1. Prepare instead of refuse

- [x] 1.1 `prepareAssignment` fires `FireClear` when a worker is past `ContextFullFraction`, ahead
      of `compactIfDue` — a clear wins over a compaction whenever both would apply.
- [x] 1.2 `claimNext` no longer refuses a full worker; `waitForNextTask` and `CmdNext` no longer
      short-circuit into `DirFull` ahead of it.
- [x] 1.3 `DirFull` is deleted.

## 2. Fullness stops meaning "needs a human"

- [x] 2.1 `parkedByTheHub` no longer exempts a full worker from the stall nudge.
- [x] 2.2 `agentBlocked` no longer reports fullness as a reason nothing is assigned.
- [x] 2.3 The board's `full` status (`overlayFullness`, `api.StatusFull`) and its counting in
      `AgentNeedsUser` are removed; `ContextTokens`/`ContextWindow` stay on `AgentView` unchanged.
- [x] 2.4 Front-end text naming the old remedy (`agent clear-context <name>` for a full agent) is
      updated in the CLI's needs-you summary, the TUI's status colouring, and the README's status
      table.

## 3. Verify

- [x] 3.1 `make verify` passes.
- [x] 3.2 `openspec validate --all` passes for this change.
