# Escalate: an agent stops on a decision only the user can make

## Why

An agent that cannot go on without a decision that is the user's has no way to say so. The system
prompt already tells it what to do in that position — "say what you need in one line and carry on
with whatever part you can" — and that is all it can do: say it into a pane nobody is looking at.
So it does one of two things instead. It guesses, which is how a wrong assumption becomes a merged
PR and is then defended by the next agent that reads the code. Or it stops, and a stopped agent is
indistinguishable from one with nothing to do: same idle status, same silence, same board row.
Neither is recoverable except by a human happening to open that pane.

Every other state where only a human can move an agent on is already named and marked — blocked,
signed-out, full, stalled (`sd-7ffc82`). The one an agent would raise deliberately, about the work
itself, was missing. That is the state most worth having, because it carries a question: the other
four say "something is wrong here", and an escalation says what to decide.

The materials were all in place. `registry.Command.Blocked` already reports why the state machine
holds a verb back from a caller, in the agent's own words rather than as a failure. Agents can
comment on their tasks (`sd-1c3041`), so a question can be recorded where the next reader of the work
will find it. The board already has a rule for "cannot move without the user" and a marker that
counts it. What was missing was a state to hang them on.

## What changes

- A verb pair, open to every role: `escalate "<what needs deciding>"` stops the agent, `resume`
  releases it. Any agent can meet a decision that is not its to make; a reviewer or planner stuck on
  one is as stuck as a worker.
- The question is MANDATORY. An escalation with no question tells the user only that something is
  wrong, which is the part the board already shows them.
- The state is durable, in `agent_state.escalation`, because a hub restart that dropped it would
  leave an agent refused by every work verb with nothing to say why. It is written only by
  `SetEscalation`/`ClearEscalation` — never by `SetState`, whose callers each build a fresh
  `AgentState` from the columns they care about and would clear it on the next phase change.
- The question is recorded twice over: in the agent's activity log, and as a comment on the task it
  holds. The comment is the durable half — it is what a human or the next agent reads when they open
  the work, long after the session that asked is gone.
- While escalated, every verb that LANDS work is refused, in every role: `next`, `submit`,
  `checkpoint`, `contribute`, `approve`, `reject`, and the planner's `openspec` — its `openspec
  submit` is a worker's submit in different dress, so holding one without the other would leave the
  same act open under a second name. Each refusal quotes the agent's own question back and names
  `resume`, reusing the existing `Blocked` mechanism rather than a second gate.
- Reads are NOT refused — `status`, `task`, `show`, `prs`, `log` and the whole `git` read/restore
  subset stay open. An agent that has just been given a decision has to re-read the task and its own
  diff to act on it; denying that makes the answer unusable.
- Proposing is not landing, so `create-task`, `edit-task`, `prioritise-task` and `reopen-task` stay
  open: the user rules on each before it becomes work, and a planner tidying the backlog while it
  waits changes nothing anyone acts on. Neither is withdrawing: `revoke` stays open because an agent
  that escalated on realising its submitted branch rests on a guess needs the verb that takes that
  branch back. The line is stated on `heldByEscalation` and in the spec, so the next verb's author
  knows which side of it they are on.
- Every message about the hold says exactly what it holds, from one shared string
  (`workflow.EscalationHold`). "Everything is refused" said where something is not spends the agent's
  trust in every other hub message — and it was false for a planner in the first cut of this change.
- The agent clears its own escalation, because only it knows whether it has understood the answer.
  The user can clear one too, from the host (`sindri agent resume`, the TUI's escalation line): an
  agent may be deleted, restarted, or simply wrong that it was blocked, and an escalation nobody can
  clear is a stuck agent by another name.
- `escalated` joins the states that need the user (`api.AgentNeedsUser`), so the Agents handle's
  marker and `sindri agent list` count it without a second marker being invented. It counts even for
  a retired agent — retirement is excluded from that rule because it REACHES full and stalled by
  itself, and nothing about winding an agent down asks a question in its name.
- It is a separate word from `blocked`, which `AgentView.Runtime` already carries for Claude sitting
  at a prompt. Both need a human; the remedies differ — a runtime block is answered in the pane, an
  escalation is answered and then the agent must resume — so folding them into one word would leave
  the user unable to tell which action is required.
- The question is readable without attaching: on the board as `AgentView.Escalation`, in `sindri
  agent info`, in the closing line of `sindri agent list`, and in the TUI's agent detail. Several
  escalations can then be triaged before deciding which to sit down with.
- The stall nudge exempts it, as it already exempts a retired or full agent: an escalated agent is
  idle BY INSTRUCTION, and `sd-521867` records what happens otherwise — an agent told "just wait" was
  nudged four times for waiting.
- The directive answers an escalated agent with its own question, on every ask. That is what makes a
  relaunch survivable: a rehydrated agent remembers nothing of escalating and would try to carry on.

## Impact

- **Specs:** `05-workflow` (the verb pair, the hold, the recording, the directive), `view-workers`
  (the marker set gains a state; the CLI's rendering of the question), `view-tui` (the question in
  the agent detail, and the user's release from it).
- **Code:** `internal/hub/store/workflow.go` (the column and its two writers),
  `internal/hub/escalate.go` (new — the verbs and the recording), `internal/hub/commands.go` (the
  registry entries and `heldByEscalation`), `internal/hub/registry/registry.go` (`Caller.Escalation`),
  `internal/hub/state.go` (`overlayEscalation`), `internal/hub/workflow/prompts.go` (the directive,
  the refusals, the system-prompt line), `internal/hub/workflow/task.go` (the directive path),
  `internal/hub/workflow/stall.go` (the nudge exemption), `internal/api` (`attention.go`, `board.go`),
  `internal/hub/server.go` + `internal/client/client.go` (`POST /agent/resume`),
  `internal/ui/cli/agent.go`, `internal/ui/tui/tab_agents.go`.
- No existing state changes meaning, and an agent that never escalates meets none of this. The one
  behaviour that changes for everybody is the system prompt, which now names the verb — without that
  the capability exists and nothing reaches for it.
