# Workflow — delta

## ADDED Requirements

### Requirement: An ending carries the reason work ended

Ending a unit of work SHALL name which event ended it — its PR merged, a worker finished the subtask
it was on, the user closed it, or the user scrapped it — and the task's own source SHALL decide what
to do from that reason. A source SHALL NOT be handed one flag covering several events and left to
pick a single behaviour for all of them.

#### Scenario: A finished subtask and a merged PR are told apart

- **WHEN** a worker finishes the subtask it is on inside a feature
- **THEN** its source is told that is what happened, and may act differently than it would for the
  same task's PR merging

#### Scenario: A scrap and a close are told apart

- **WHEN** the user scraps a task, and separately when the user closes one
- **THEN** each reaches the source as its own reason rather than as the same flag

### Requirement: An openspec change is done when its PR merges

A change behind an `os-` task SHALL be archived when that task's pull request merges, and SHALL NOT
be archived because the task was closed or because a worker finished the subtask it was on. A
scrapped change SHALL be removed, since no merge is coming to end it.

#### Scenario: A checkpoint does not retire the change

- **WHEN** a worker finishes a subtask whose task is an openspec change, before submitting it
- **THEN** the change stays live, and the task stays in the backlog for the rest of its own tasks

#### Scenario: A merge archives the change

- **WHEN** the pull request for an `os-` task merges
- **THEN** its change is archived at that point

### Requirement: A source refuses to end visibly unfinished work, and says so

A source that can see the work behind a task is incomplete SHALL refuse to end it, and SHALL state
the completion it refused on — an openspec change knows how many of its own tasks are ticked. It
SHALL NOT end the work silently.

#### Scenario: An incomplete change is not retired quietly

- **WHEN** something would end an openspec change whose own task list is not fully ticked
- **THEN** it is refused, and the refusal names how many of the change's tasks are done out of how
  many — the same count the backlog listing already shows

### Requirement: A committing verb is named for what it commits to

A verb that ENDS a unit of work SHALL be named as an ending, and a verb that lands work while
KEEPING the unit SHALL be named as an interim step. A verb's name and its advertised description
SHALL both say which of the two it is.

#### Scenario: The terminal verb reads as terminal

- **WHEN** a worker reads the verbs available to it inside a feature
- **THEN** the one that closes the subtask it is on reads as an ending, and the one that lands work
  mid-task reads as an interim step, without the worker having to try one to find out
