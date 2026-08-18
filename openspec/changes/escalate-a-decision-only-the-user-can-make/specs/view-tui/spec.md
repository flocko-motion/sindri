# view-tui — delta

## ADDED Requirements

### Requirement: The agent detail carries an escalation, and releases it

The Agents tab's detail SHALL show an escalated agent's question, beside the status
word that says it is escalated, so the user reads what is being asked without
attaching to the pane. The line SHALL be absent when there is no escalation: a field
shown empty reads as a question nobody recorded.

That line SHALL be the user's release of the escalation, so the action is offered where
the question is read rather than through a hotkey of its own — it means something only
on an escalated agent, and this is the one place such an agent is looked at. It SHALL
confirm before clearing, since clearing discards the agent's own account of why it
stopped, and the agent's own resume is the ordinary path.

#### Scenario: Reading what an agent asked

- **WHEN** the user selects an escalated agent
- **THEN** the detail shows the question in full, alongside its `escalated` status

#### Scenario: Releasing an agent from the detail

- **WHEN** the user acts on the escalation line
- **THEN** a confirmation is asked, and confirming clears the escalation so the agent
  carries on

#### Scenario: An agent that has escalated nothing

- **WHEN** the user selects an agent with no escalation
- **THEN** no escalation line is shown
