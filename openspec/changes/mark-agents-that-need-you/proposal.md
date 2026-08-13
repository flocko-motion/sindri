# Mark the agents that cannot move without you

## Why

An agent that is blocked at a prompt, signed out, full, or stalled looks alive on the board: it has
a name, a task, and a status word among fourteen others. It holds that task and makes no progress,
and nothing says so anywhere a human is looking. The recent gopls trouble was exactly this — agents
stopped at a permission prompt, discovered because somebody happened to open the tab.

The Tasks handle has carried a `(N!)` marker for work awaiting a verdict since the same problem was
noticed about the backlog. The Agents handle is the other half: both count what waits on the user,
and there was no reason for one to be visible from every tab and the other from one.

This is the third such marker (Tasks, PRs, Agents), which settles where the rule belongs. The hub's
section model is already "the single source of truth for which views exist and the badge each
shows"; a third `if s.Key == "…"` in the view would have been the third place to keep in step.

## What changes

- `api.Section` gains `Attention` beside `Count`: how many of that section's rows wait on the user.
  `hub/commands` computes it per section — the Tasks recipe is the verdict count that already
  existed, the Agents recipe is the rule below — and `Resolved` sends both numbers. A section with
  nothing that can wait on a human leaves the recipe nil.
- The resolved sections ride on `BoardState`, so a front-end that may not link the hub still renders
  hub-decided numbers rather than deriving its own. The TUI draws every handle in one loop.
- The rule: an agent counts when its state RESOLVES ONLY IF A HUMAN ACTS (`api.AgentNeedsUser`).
  Today that is `blocked`, `signed-out`, `full` and `stalled`. Whoever adds the next status asks
  that question of it, rather than matching the shape of these four.
- Idle never counts, and this is the shared half of the definition with the idle-agent observer:
  idle with no work available is healthy; idle beside work it could claim is a dispatch fault the
  hub nudges; idle because a human must act is this set, which is marked and never nudged. An
  `api-error` is the hub's to resend — once resending stops working the screen stands still and the
  board says `stalled`.
- Retired is excluded ahead of the status, because it reaches the states that count: retirement
  withholds new work rather than stopping the agent, so a retired one fills its context and reads
  `full`, or holds work and stands still and reads `stalled`. Since retiring a full worker is how
  one is ordinarily wound down, counting it would park the marker on the handle until the agent was
  deleted. `sd-521867` exempted parked agents from the stall nudge on the same reasoning, and the
  two rules have to say one thing about the same agent.
- `sindri agent list` marks each such row and closes with a line naming them, so a CLI user learns
  the fleet has stopped on them without attaching to every pane.

## Impact

- Specs: `hub` (the section model gains the attention count), `view-tui` (the marker on every
  handle), `view-workers` (the rule, and the CLI's rendering of it).
- Code: `internal/api` (`attention.go`, `section.go`, `board.go`), `internal/hub/commands/sections.go`,
  `internal/hub/state.go`, `internal/ui/tui/tui.go`, `internal/ui/cli/agent.go`.
- An agent both full and stalled wears one status word and is one thing to attend to, so the count
  is of agents and cannot double. The stall nudge already exempts an agent parked by the hub, so
  nothing is prodded for a state it was told to hold.
- The glyph is unchanged (`!`) and so is its shape, `(N!)`. A user who knows the Tasks marker knows
  this one.
