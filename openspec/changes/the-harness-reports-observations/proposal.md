# The harness reports observations; the orchestrator draws conclusions

## Why

These are different kinds of statement and today they are the same sentence.

Whether a process is up, whether the pane is at a prompt, how long the screen has been unchanged,
how large the transcript is — those are facts about a box, and only the harness can see them.
Whether an agent is *stalled*, *idle*, *blocked* or *full* are judgements about work, true only in
the light of what the agent holds.

It is backwards in both directions. `workflow/stall.go` switches on the literal strings `"blocked"`,
`"signed-out"` and `"api-error"` scraped off the pane; `hub` folds the same word into a board status
in `overlayRuntime`. Meanwhile the whole standing observation — up, clients, the runtime word, the
pane digest, how long it has been still, whether a tool is in flight, the context fill — reaches the
workflow through three booleans (`AgentAlive`, `AgentUp`, `AgentIdle`). Everything else is either
re-derived or passed as an untyped string.

The reason this is worth doing is debugging. With the observation a value in its own right, what the
hub SAW can be shown beside what it CONCLUDED, and a wrong answer is one question — was the evidence
wrong, or the rule?

## What Changes

**One observation value crosses the seam.** The watchdog's `liveness` becomes a named observation the
harness returns from `Observe(project, name)`: up, clients, the session's own runtime word, pane
digest, still-since, tool-since, fill, and the time it was taken. `AgentAlive`, `AgentUp` and
`AgentIdle` go — each was a boolean cut out of this value.

**The words move to the orchestrator, by name.** `Stalled` reads named fields instead of matching
strings. `overlayRuntime` folds an observation rather than a word. The naming rule that settles
future cases: the harness may say *what it saw*, never *what it means*. `Idle` is the one to watch —
"at an empty prompt" is an observation, "has nothing to do" is a conclusion, and one identifier has
been carrying both.

**The interfaces split.** `workflow.Deps` becomes `Harness` (`Observe`, `Say`, `Clear`, `Compact`,
`SetModel`, `Interrupt`, `Start`, `Container`) and `Deps` (the project's facts, the board, the task
thread, the mailbox). The split is the check on every future change: a method that names a task
belongs on the second, a method that names a keystroke belongs on the first, and a method that names
both is the next thing to pull apart.

**Delivery stops asking the ruleset for permission.** `hub.Deliver` calls `workflow.WakeRefusal`
while `workflow` reaches delivery through `deps.Deliver` — a cycle through the composition root,
and `.Regardless()` exists only because the gate sits where the caller cannot see it. The decision
moves to whoever composes the message; delivery carries out what it is given and reports what
happened.

## Impact

- Specs: `agent-runtime` gains the observation contract and the harness's ignorance of tasks.
- Code: `internal/hub/watchdog.go`, `internal/hub/state.go`, `internal/hub/deliver.go`,
  `internal/hub/wiring.go`, `internal/hub/workflow` (`engine.go`, `stall.go`, `explain.go`,
  `delivery.go`), `internal/arch`.
- No rule changes meaning. This moves where a rule is evaluated and what it reads.

## Where this sits

Last of three, and it depends on both. `a-harness-command-answers` removes the machinery that would
otherwise have to be carried across the split. `one-surface-for-what-is-allowed` builds the one
place the conclusions live — without it, the words moved here would land somewhere temporary and be
moved again.
