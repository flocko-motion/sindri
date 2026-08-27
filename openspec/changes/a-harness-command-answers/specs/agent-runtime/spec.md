# Agent runtime — delta

## ADDED Requirements

### Requirement: A harness command blocks and answers

Every command that drives the harness — the sandbox with an agent inside — SHALL carry its operation
out to completion before returning, and SHALL answer with success, a timeout, or a stated error. It
SHALL NOT return once the input is typed and finish the work in the background. This covers clearing
a session, compacting it, and changing the model it runs on.

A harness command SHALL NOT take a follow-up message to queue behind itself. The caller, having an
answer, sends whatever comes next through the ordinary delivery path.

A harness command SHALL be bounded by a timeout, and a timeout SHALL be reported to its caller as a
failure of that command rather than recorded and dropped.

#### Scenario: A clear that takes effect answers success

- **WHEN** the orchestrator clears an agent's session and the session's context is observed to have
  been discarded
- **THEN** the command returns success, and the orchestrator sends the agent its next instruction
  itself

#### Scenario: A clear that never takes effect answers a failure

- **WHEN** the orchestrator clears an agent's session and the session's context is never observed to
  fall within the command's timeout
- **THEN** the command returns a timeout failure naming the agent, and the orchestrator is the one
  that decides what happens next — rather than the agent being left with neither a cleared session
  nor an instruction

#### Scenario: No follow-up rides along with the command

- **WHEN** any harness command is issued
- **THEN** it carries no message to be delivered after it, and nothing is injected into the session
  by the command beyond the operation itself
