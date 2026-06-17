# Workflow — delta

## MODIFIED Requirements

### Requirement: The worker loop

A worker SHALL run a loop of: receive a task (injected by the hub) → implement,
test, commit → register the branch for merge → go idle. Registering for merge
SHALL return immediately; the worker SHALL NOT block waiting for the verdict and
SHALL NOT poll. The hub SHALL wake the worker by injecting the next task or the
verdict when ready. Idle is the worker's resting state, and a long wait is
expected.

#### Scenario: One iteration

- **WHEN** a worker finishes a task and registers it for merge
- **THEN** the call returns at once and the worker goes idle until the hub injects
  the next task or a verdict

#### Scenario: Queue empty

- **WHEN** there is no open task for a worker
- **THEN** the worker simply stays idle; the hub injects a task when one appears

### Requirement: Communication via comments

Messages to an agent SHALL be delivered by the hub injecting them into the
agent's session, each stamped with its source. A human reaches an agent via the
hub-mediated `tell` channel; another agent reaches it only by acting on a shared
object whose consequence the hub routes. The agent SHALL see these as tagged
lines in its single input stream.

#### Scenario: Human nudge

- **WHEN** a human sends a message to an agent
- **THEN** the hub injects it into that agent's session tagged `[user]`

#### Scenario: Reviewer feedback

- **WHEN** the reviewer rejects an agent's PR with feedback
- **THEN** the hub routes the feedback to the owning agent's session tagged
  `[reviewer]`
