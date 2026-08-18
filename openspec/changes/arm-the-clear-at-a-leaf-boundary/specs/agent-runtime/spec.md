# Agent Runtime — delta

## MODIFIED Requirements

### Requirement: A human can clear a full worker's session at a leaf boundary

Sindri SHALL provide a way to send Claude Code's own `/clear` into an agent's live
session and re-serve its current directive afterward, so the agent resumes exactly as
it would after a fresh launch. Clearing SHALL happen only at a LEAF BOUNDARY: a held
task's memory of the worktree's contents would go stale silently, since only the
task's own next claim resets the worktree.

Asking for a clear SHALL therefore ARM one rather than perform it. An agent already at
a boundary is cleared at once; one holding work keeps the arming until it reaches its
next boundary — finishing its task, checkpointing within a feature, or delivering its
verdict. The user chooses whether, never when: when is decided by the agent's state.

A boundary is holding no leaf task and owing no verdict. A feature in hand is not
work a clear cuts into, since the clear fires between its subtasks. A planner and a
coauthor hold neither, so they are always at one.

An armed agent SHALL be handed no new leaf work until the clear lands, and SHALL be
told that is why — otherwise the clear would arrive into work served in the meantime,
meeting the very boundary rule it was waiting for. This SHALL hold on every path that
assigns work, including each way a review is handed out: one gated and one not leaves
the arming deferrable for ever by a steady queue, and leaves an assignment able to land
between the boundary check and the clear. An armed clear SHALL take
precedence over the notice that a full agent is retired from assignment: that notice
asks for a human to act, and one has.

The arming SHALL be durable, surviving a hub restart: an arming that evaporated would
leave the user believing it was set. It SHALL be visible wherever agents are listed
and in an agent's detail, since a toggle whose state cannot be seen is worse than
none. Asking again SHALL take the arming back, and taking it back SHALL NOT be
confirmed — cancelling a destructive action is not itself destructive. Invoking the
action IS the confirmation of arming; no further prompt is required of a CLI user,
while a full-screen interface SHALL confirm because the keystroke is one press.

#### Scenario: Clearing an agent already at a boundary

- **WHEN** a human clears an agent that holds no leaf task and owes no verdict
- **THEN** `/clear` is sent into its session and its directive is re-served, landing
  it back at "ask for work"

#### Scenario: Clearing a worker mid-task arms it

- **WHEN** a human clears an agent that currently holds a leaf task
- **THEN** the clear is armed, the user is told which work it will fire after, and the
  agent keeps working until it reaches that boundary

#### Scenario: An armed agent is given no work meanwhile

- **WHEN** an armed agent asks the hub for work
- **THEN** it is told a clear is about to land and is assigned nothing, rather than
  reading an empty queue as "nothing to do"

#### Scenario: The arming survives a restart

- **WHEN** the hub restarts between the arming and the boundary
- **THEN** the clear is still armed and still fires at the next boundary

#### Scenario: An armed reviewer is passed over

- **WHEN** a PR needs a reviewer and the only free one has a clear armed
- **THEN** it is not assigned that review, on the request path as on the sweep

#### Scenario: A clear that cannot be performed leaves nothing behind

- **WHEN** a human clears an agent at a boundary whose session cannot be reached
- **THEN** they are told it failed and no arming is recorded, so the agent is not
  silently withheld from work by a clear that never happened

#### Scenario: Taking the arming back

- **WHEN** a human asks again for an agent whose clear is already armed
- **THEN** the arming is removed without a confirmation, and the agent is served work
  again

#### Scenario: A full agent is cleared and returns to service

- **WHEN** a clear is armed for an agent retired by its own full context
- **THEN** it fires at the boundary the agent is already at, and the agent is handed
  work again rather than told to wait for a human
