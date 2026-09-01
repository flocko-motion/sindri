# Agent runtime — delta

## ADDED Requirements

### Requirement: The harness reports observations, never conclusions

The harness SHALL report what it can see of an agent's box as evidence — whether the process is up,
how many clients are attached, what the session's own runtime word says, whether the displayed
content has changed and for how long it has not, whether a tool call is in flight, and how large the
session's context is — carried as one observation value stamped with when it was taken.

The harness SHALL NOT report a judgement that depends on what the agent holds. "Stalled", "blocked",
"needs the user", and "full" are the orchestrator's words, defined against the observation together
with the agent's held work, and SHALL be defined only there.

The observation SHALL be readable as itself, so that what the hub saw can be shown beside what it
concluded and the two can be told apart.

#### Scenario: Raw evidence crosses the seam intact

- **WHEN** the orchestrator needs to know an agent's situation
- **THEN** it receives the observation with its evidence and its timestamp, rather than a set of
  booleans each of which has already discarded the reasoning behind it

#### Scenario: A conclusion is not smuggled across as a word

- **WHEN** the session's own runtime reports a state such as being blocked on a prompt or signed out
- **THEN** that word crosses as evidence the orchestrator interprets, and the rule that turns it
  into "this agent is stalled" or "this agent needs the user" lives with the orchestrator

#### Scenario: Being at a prompt is not the same as having nothing to do

- **WHEN** an agent is observed sitting at an empty prompt
- **THEN** the harness reports that it is at a prompt, and whether that means the agent is idle —
  free to be given work — is decided by the orchestrator against what the agent holds

### Requirement: The harness knows nothing of tasks

The harness interface SHALL name only the agent and its box — observing it, saying something,
clearing, compacting, changing model, interrupting, starting. It SHALL NOT name a task, a PR, a
review, a backlog or a verdict, and SHALL NOT consult the orchestrator's rules to decide whether to
carry out a command it was given.

#### Scenario: A harness command is not second-guessed

- **WHEN** the orchestrator issues a harness command for an agent
- **THEN** the harness carries it out and reports the outcome, without asking whether the agent's
  held work makes the command appropriate

#### Scenario: Whether to wake an agent is the sender's decision

- **WHEN** a message is composed for an agent that the rules say is not worth waking
- **THEN** that judgement is made where the message is composed, and the delivery path carries out
  what it is given and reports what happened
