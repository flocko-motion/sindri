# hub — delta

## ADDED Requirements

### Requirement: A task that gains a child under its worker extends that work

Adding a child to a task an agent is already working SHALL raise the bar for that agent's pull
request rather than stranding the agent or merging over the new work.

Where the agent holds the task as a LEAF, it SHALL come to hold it as a FEATURE: the same agent, on
the same branch, with the subtasks handed out one after another and ONE pull request covering the
whole hierarchy. A hierarchy changes the unit under review, never who puts it up.

The promotion SHALL assign nothing itself. The agent SHALL be parked on the feature and the ordinary
directive SHALL pick the subtask, so every question about what may be handed out is asked in one
place — and asked after the approval state a proposal receives a moment after the task itself, which
an assignment made at promotion time would read before it existed.

Where the agent holds the task inside a feature already, nothing about what it holds SHALL change:
the child is in the same feature on the same branch, and the subtask loop already reaches work at
any depth. No further nesting mechanism SHALL be built, and no edit SHALL be refused for depth.

The agent SHALL be told what was added and that its pull request now covers it. Its unit of work
changed shape by someone else's act, and being refused at its next checkpoint is not how it should
find out.

A checkpoint on a task that has open children SHALL record the work, leave that task OPEN, and hand
over the next open leaf. It SHALL NOT refuse: the agent can neither close the child, re-parent it,
nor rule on it, so a refusal leaves it with no exit but a human noticing.

No task SHALL be closed while it has an open child, by merge or by any other path. A merge of a
pull request whose task has open children SHALL land the branch as a MILESTONE — the task stays
open and its worker stays on it — rather than closing it. A task is UNFINISHED in every status but
a terminal one, so a child being WORKED counts as plainly as one not yet started.

Every open child blocks. Work meant not to block SHALL be ruled on rather than parked silently: a
rejected child blocks neither claiming nor completion, is recorded with its reason, and can be
reversed. There SHALL be no way to add a child that silently fails to block, since that is how a
parent comes to be closed over real work.

#### Scenario: A leaf task that gains a child

- **WHEN** a child is added under a task an agent is working as a single task
- **THEN** the agent holds it as a feature on the same branch, is told what was added and that its
  pull request now covers both, and is handed the new work by its next directive

#### Scenario: A gained child still awaiting a verdict

- **WHEN** the child added is one the user has not yet approved
- **THEN** it is not handed to the agent, and the feature is held open by it until the user rules

#### Scenario: A subtask inside a feature gains a child

- **WHEN** a child is added under a subtask of a feature an agent holds
- **THEN** what the agent holds does not change, it is told, and its checkpoint carries it on to the
  new work rather than refusing over it

#### Scenario: The parent that gained work is not closed by a checkpoint

- **WHEN** a checkpoint is run on a task that has open children
- **THEN** the work is recorded, that task stays open, and the next open leaf is handed over

#### Scenario: Submitting a task that grew

- **WHEN** an agent submits a task that gained a child while it was not running
- **THEN** no pull request is opened, the agent comes to hold the task as a feature, and it is told
  what it holds and where the new work is

#### Scenario: A merge never closes a task over open children

- **WHEN** a pull request merges whose task has an open child
- **THEN** the branch lands, the task stays open, and its worker keeps it as a feature

#### Scenario: A child that should not block

- **WHEN** the user does not want an added child to hold the work in flight
- **THEN** rejecting it releases the parent, with the reason on record, and there is no way to add a
  child that fails to block silently
