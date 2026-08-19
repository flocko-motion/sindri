# Workflow — delta

## ADDED Requirements

### Requirement: A worker past its context threshold is cleared and assigned, not refused

The automatic assigner SHALL claim a leaf task or a container for a worker whose current context
size is at or past a fixed threshold exactly as it would for any other worker. Once claimed, the
worker's session SHALL be cleared (its live context discarded, Claude Code's own `/clear`) rather
than compacted, before its directive is delivered — a session that far past the threshold is not
worth summarizing. A worker with no context measurement yet SHALL NOT be treated as past the
threshold. A model switch the worker's tier requires SHALL still take priority over a clear.

#### Scenario: A full worker is claimed for and then cleared

- **WHEN** an idle worker whose context is past the threshold asks for its next task, and an open
  leaf or an unheld container exists
- **THEN** the assigner claims it for the worker, then clears the worker's session before its
  directive is delivered, rather than refusing the claim

#### Scenario: A full worker is not left waiting on a human

- **WHEN** an idle worker whose context is past the threshold asks for its next task
- **THEN** it is not told to wait for a human to clear it; the clear fires automatically as part of
  being handed the task

#### Scenario: An unmeasured worker is assigned normally

- **WHEN** an idle worker with no context measurement yet asks for its next task
- **THEN** the assigner claims for it exactly as it would for any worker under the threshold

#### Scenario: A model switch still takes priority

- **WHEN** a worker is both past the context threshold and due a model switch for its tier
- **THEN** the model switch fires; the clear this requirement describes does not pre-empt it

### Requirement: A worker mid-task is never parked or excused on fullness alone

Fullness SHALL NOT be a reason an agent is treated as deliberately parked, exempted from the stall
nudge, or reported as blocking assignment. A worker already holding a task or a container SHALL be
nudged like any other stalled worker regardless of how full its context is, since nothing clears an
agent's context while it holds work — only a fresh, idle ask does.

#### Scenario: A full worker mid-task is still nudged when it stalls

- **WHEN** an agent holding a task is past the context threshold and its screen has stood still past
  the stall dwell
- **THEN** it is nudged exactly as a worker under the threshold would be

#### Scenario: Fullness is not reported as a reason nothing is assigned

- **WHEN** something asks why an idle agent past the context threshold is being handed nothing
- **THEN** the answer does not name fullness as the reason, since the next ask clears and assigns it
