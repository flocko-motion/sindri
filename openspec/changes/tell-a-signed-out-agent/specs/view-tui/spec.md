# view-tui — delta

## ADDED Requirements

### Requirement: Telling a signed-out agent is a choice, not an error

Where the user sends a message to an agent the board reports as signed out, the TUI SHALL offer the
answers rather than return the hub's refusal: restart the agent and send, send regardless, or
cancel. The restart SHALL lead the answers, being the remedy that works, and cancel SHALL sit under
the cursor as it does in every other confirm. The modal SHALL say what a restart does, since that is
why it is offered.

Every other message SHALL go as it always did, the hub's guard standing behind it.

#### Scenario: The message is held for an answer

- **WHEN** the user sends a message to an agent that reads signed out
- **THEN** the choice opens with restart-and-send, send-anyway and cancel, and the message is held
  until one is picked

#### Scenario: An agent that reads fine

- **WHEN** the user sends a message to any other agent
- **THEN** it is delivered without a prompt
