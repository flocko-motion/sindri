# view-workers — delta

## ADDED Requirements

### Requirement: A context-full agent reads as full only where that explains it

An agent's status SHALL read `full` only where the agent holds no work — no task, no feature and
no PR — and its status would otherwise be `idle`. Fullness is the reason such an agent is being
passed over for new work, so that is the one place the word informs.

Where the agent holds work or is otherwise occupied, the status SHALL keep the word it would
otherwise carry, including `working`, `blocked`, `submitted`, `down` and `stalled`. An agent that
is both full and stalled SHALL read `stalled`, which is the more actionable of the two.

An agent's context fill SHALL remain readable as its numeric fields whatever its status reads, so
that suppressing the word never hides the condition.

#### Scenario: An idle agent explains itself

- **WHEN** an agent past the fullness threshold holds no task, feature or PR
- **THEN** its status reads `full`

#### Scenario: A working agent keeps working

- **WHEN** an agent past the fullness threshold is working on a task
- **THEN** its status reads `working`, and its fill is still readable from its context fields

#### Scenario: A stalled agent stays stalled

- **WHEN** an agent is both past the fullness threshold and stalled
- **THEN** its status reads `stalled`

#### Scenario: A quiet task-holder is not idle in this sense

- **WHEN** an agent past the fullness threshold holds a task but its runtime reads idle
- **THEN** its status does not read `full`
