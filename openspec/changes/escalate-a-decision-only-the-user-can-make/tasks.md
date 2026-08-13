# Tasks

## 1. The durable state

- [x] 1.1 `agent_state` gains an `escalation` column (migrated for an existing store), carried on
      `store.AgentState` so every reader of the state has it — the command surface, the board, the
      directive.
- [x] 1.2 Written only by `SetEscalation`/`ClearEscalation`. `SetState` leaves the column alone and
      says why: its callers each build a fresh `AgentState`, so writing it from the struct would clear
      a live escalation on the next phase change.

## 2. The verbs

- [x] 2.1 `escalate <what needs deciding>`, open to every role; the question is required and a
      questionless call records nothing.
- [x] 2.2 The question is recorded in the activity log and, where the agent holds work, as a comment
      on that task (the subtask inside a feature, matching where its own `comment` writes).
- [x] 2.3 A failed comment is reported host-side, not fatal — the escalation is what stops the agent,
      and the log already carries the question.
- [x] 2.4 `resume`, held back when nothing is escalated, and never held back when one is.
- [x] 2.5 Re-escalating replaces the question, so a badly-put one can be re-put.

## 3. The hold

- [x] 3.1 `heldByEscalation` wraps each landing verb's own gate — the existing
      `registry.Command.Blocked` mechanism, not a second gate.
- [x] 3.2 The line is WHAT LANDS WORK, and it is written down where the next verb's author will read
      it (on `heldByEscalation`, and as the spec's rule): shut are `next`, `submit`, `checkpoint`,
      `contribute`, `approve`, `reject` and the planner's `openspec`; open are the reads, the records,
      the proposals (`create-task`, `edit-task`, `prioritise-task`, `reopen-task`) and `revoke`,
      which withdraws work rather than landing it.
- [x] 3.3 `openspec` is held: a planner's `openspec submit` is a worker's submit in different dress,
      and holding one without the other leaves the same act open under a second name.
- [x] 3.4 The refusal quotes the agent's own question and names `resume`.
- [x] 3.5 Reads stay open: status, task, show, prs, log, git.
- [x] 3.6 `registry.Caller` carries the question rather than a flag, so the refusal can quote it.
- [x] 3.7 Every message about the hold says exactly what it holds, from one shared string
      (`workflow.EscalationHold`) — an agent that finds a hub message false stops trusting the rest.

## 4. What the agent is told

- [x] 4.1 The directive answers an escalated agent with its own question, ahead of every role's own
      directive — the rehydration case, since a relaunched agent remembers nothing of asking.
- [x] 4.2 Resuming returns it to the work it still holds.
- [x] 4.3 The stall nudge exempts it, ahead of the api-error retry: a resumed turn has no verb left
      that lands work.
- [x] 4.4 The system prompt names the verb — without that, the capability exists and nothing reaches
      for it.

## 5. Visibility

- [x] 5.1 `escalated` joins `api.AgentNeedsUser`, ahead of the retirement exclusion, so one marker
      counts it.
- [x] 5.2 `overlayEscalation` puts the word on the board over the runtime and the stall, under
      not-up and signed-out.
- [x] 5.3 The question rides on `AgentView`, so a front-end renders it rather than fetching it.
- [x] 5.4 `sindri agent list` quotes it in the closing line and names the release; `sindri agent
      info` prints it in full.
- [x] 5.5 The TUI's agent detail shows it, and ENTER on that line clears the escalation after a
      confirm.

## 6. The user's release

- [x] 6.1 `POST /agent/resume` → `client.ResumeAgent` → `sindri agent resume <name>` and the TUI's
      escalation line, so both front-ends reach it.
- [x] 6.2 Clearing nothing is a no-op, so neither caller has to check first.

## 7. Verify

- [x] 7.0 EVERY ROLE's whole surface, partitioned: each verb that lands work refuses with the
      agent's own question, and nothing else refuses because of the escalation. The reviewer's
      `approve`/`reject` and the planner's `openspec` are unreachable from a worker's surface, so a
      worker-only test cannot see them — which is how `openspec` was missed.
- [x] 7.1 The hold and the reads, verb by verb, off the advertised surface.
- [x] 7.2 A questionless escalation records nothing.
- [x] 7.3 The question in the log and on the task; the release recorded too.
- [x] 7.4 Durability against an unrelated phase write.
- [x] 7.5 The directive on a relaunch, and the nudge exemption (including a cut-off turn).
- [x] 7.6 The status word's precedence, both ways, and the marker — including a retired agent's
      question, which still counts.
- [x] 7.7 The front-ends: the question quoted in the CLI summary, and the TUI detail line that
      releases it.
- [x] 7.8 A held verb EXECUTED, not merely listed: the refusal happens and nothing it would have
      changed changed. The surface and the dispatcher read one predicate, and this is what proves it.
- [x] 7.9 `make verify` passes.
