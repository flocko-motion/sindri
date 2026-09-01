# Workflow — delta

## ADDED Requirements

### Requirement: The orchestrator acts on announced changes rather than on its own clock

The orchestrator SHALL subscribe to observation changes and decide when one arrives, rather than
asking on an interval of its own. Where it holds a timer today because nothing announced — the stall
check, the re-announcement of unread mail — that timer SHALL be removed, or kept only as a declared
backstop with its purpose stated on it.

A decision that depends on an agent's readiness SHALL read the observation the observer has
published, rather than sampling readiness again on a different schedule.

#### Scenario: An agent going idle is noticed when it happens

- **WHEN** the observer sees an agent settle into an idle state holding nothing
- **THEN** the orchestrator is told at that point, rather than at the next interval of a check of
  its own

#### Scenario: A reachable agent is told about its waiting mail

- **WHEN** an agent that was unreachable becomes reachable
- **THEN** the mail waiting for it is announced on that change, rather than after a fixed wait

#### Scenario: A remaining timer says why it remains

- **WHEN** a periodic check survives this change
- **THEN** it states what it is a backstop FOR, so a reader can tell a deliberate safety net from a
  poll nobody replaced
