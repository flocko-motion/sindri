# Workflow — delta

## ADDED Requirements

### Requirement: Nothing closes a task whose work is visibly unfinished

The close path SHALL ask a task's own source whether the work behind it is finished, and SHALL
refuse to close it when the source says it is not. The refusal SHALL state the measure the source
refused on — an openspec change knows how many of its own tasks are ticked, and that count is
already what the backlog listing shows.

A source that cannot tell SHALL say so, and the close proceeds; the check adds a way to report a
fact only the source can see, and grants no source its own idea of when a task is done.

The refusal SHALL reach a calling agent as an ordinary answer it can act on, never as an internal
failure, since finishing the remaining work or explaining why it no longer applies is exactly what
the agent should do next.

#### Scenario: An unfinished change is not closed

- **WHEN** something would close a task whose openspec change has unticked tasks of its own
- **THEN** the close is refused and the reply names how many of the change's tasks are done out of
  how many

#### Scenario: The refusal does not stop the agent

- **WHEN** a worker's verb is refused because the work behind its task is unfinished
- **THEN** it is told what remains and carries on, rather than being escalated and stopped

#### Scenario: A source with nothing to report closes as before

- **WHEN** a task whose source cannot measure completion is closed
- **THEN** it closes exactly as it does today

### Requirement: A committing verb is named for what it commits to

A verb that ENDS a unit of work SHALL be named as an ending, and a verb that lands work while
KEEPING the unit SHALL be named as an interim step. A verb's name and its advertised description
SHALL both say which of the two it is.

#### Scenario: The terminal verb reads as terminal

- **WHEN** a worker reads the verbs available to it inside a feature
- **THEN** the one that closes the subtask it is on reads as an ending, and the one that lands work
  mid-task reads as an interim step, without the worker having to try one to find out
