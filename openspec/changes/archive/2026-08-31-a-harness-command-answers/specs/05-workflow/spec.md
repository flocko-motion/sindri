# Workflow — delta

## ADDED Requirements

### Requirement: The orchestrator sequences a session reset itself

The orchestrator SHALL issue a harness command, wait for its answer, and then deliver the following
instruction itself through the ordinary delivery path, wherever it needs an agent's session reset —
cleared, compacted, or moved to another model — before that instruction lands. It SHALL NOT hand the
instruction to the harness command to be sent on its behalf.

A harness command that fails or times out SHALL leave the orchestrator holding the decision about
what happens next, and the agent SHALL NOT be left both un-reset and un-instructed.

Because the reset completes within the call that asked for it, there SHALL be no directive whose
purpose is to answer an agent that asks for work while a reset is pending, and no stored window
marking an assignment as in-flight so that the boundary check will admit it.

#### Scenario: A cleared agent is instructed by the orchestrator

- **WHEN** the orchestrator clears an agent's session as part of handing it work
- **THEN** it waits for the clear to answer, and then delivers the agent's directive itself

#### Scenario: A failed reset is the orchestrator's to answer for

- **WHEN** the orchestrator clears an agent's session and the command answers with a failure
- **THEN** the orchestrator decides what follows — retrying, escalating, or leaving the agent as it
  was — rather than the failure being recorded where no caller reads it

#### Scenario: Nothing asks about a pending reset

- **WHEN** an agent asks the hub for its next action
- **THEN** it is never answered with a notice that a reset is about to land, because a reset in
  progress is inside a call that has not yet returned
