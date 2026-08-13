# view-workers — delta

## ADDED Requirements

### Requirement: An escalated agent is marked, and its question is readable

An agent waiting on a decision only the user can make SHALL satisfy the rule for
agents that need the user, and SHALL be counted by the same marker as the other such
states rather than by a second one invented for it. Its status word SHALL be
`escalated`.

That word SHALL be its own, distinct from `blocked`, which the runtime already carries
for a coding agent sitting at its own prompt. Both need a human, and the remedy
differs: a runtime block is answered in the pane, an escalation is answered and then
the agent must resume. One word for both would leave the user unable to tell which
action is required.

An escalation SHALL count even for a retired agent, though the other such states do
not. Retirement is excluded from them because it REACHES them by itself — a retired
agent keeps running, so it fills up or stands still — and a marker there would never
clear. Nothing about winding an agent down asks a question in its name, and a retired
agent still finishes what it holds, so its question is unanswered either way.

Two states SHALL outrank it, both saying the answer cannot be DELIVERED until
something else is fixed: an agent whose pod is not up, and a signed-out session where
nothing typed reaches the agent. Everything else it outranks, including a stalled
reading — which is the same standing still seen without the reason for it.

The question SHALL be readable without attaching to the agent's session: carried on
the board beside the agent, printed by the agent's detail view, and named in the
listing's closing line. Several escalations can then be triaged before deciding which
to sit down with, and the question — not the status word — is what the user acts on.
The listing SHALL name the release for an agent that cannot resume itself, as it names
the remedy for the other states.

#### Scenario: The fleet listing shows who asked what

- **WHEN** an agent is escalated
- **THEN** its row reads `escalated` and is marked as waiting on the user, and the
  closing line quotes its question rather than only naming the agent

#### Scenario: A retired agent's question still counts

- **WHEN** a retired agent is escalated
- **THEN** it is still counted as waiting on the user

#### Scenario: A signed-out escalated agent

- **WHEN** an escalated agent's session is signed out
- **THEN** the view says signed-out, since the answer cannot be delivered until that
  is fixed

#### Scenario: The detail view carries the question in full

- **WHEN** the user reads an escalated agent's detail
- **THEN** the question is shown in full, without attaching to its session
