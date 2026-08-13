# view-workers — delta

## ADDED Requirements

### Requirement: An armed context clear is visible and reversible

Every workers view SHALL show which agents have a context clear armed — the row and
the agent's detail alike — because the arming changes nothing observable until it
fires, and without a marker there is no way to tell an agent about to lose its session
from one carrying on.

Where the view offers the clear, it SHALL confirm arming and SHALL state WHEN it will
land, taken from the agent's state rather than asked of the user: "clears now" for an
agent at a boundary, or the work it will fire after. The same action asked again SHALL
disarm, without a confirmation.

#### Scenario: An armed agent is marked

- **WHEN** a clear is armed for an agent
- **THEN** its row and its detail both say so, and the detail says when it will land

#### Scenario: The confirm states when, and does not ask

- **WHEN** a user arms a clear from a full-screen interface
- **THEN** the confirmation states when the clear will land and offers only yes or no,
  never a choice of when

#### Scenario: Disarming asks nothing

- **WHEN** a user asks again for an agent whose clear is armed
- **THEN** the arming is removed immediately, with no confirmation
