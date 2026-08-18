# agent-runtime — delta

## ADDED Requirements

### Requirement: A signed-out pane is a question to the sender, never a veto

A message the HUB originates SHALL be refused rather than delivered into a signed-out session, and
SHALL be recorded: nothing typed at that prompt is sent — it collects in the agent's input box
unread — and no human is present to notice that it went nowhere.

A message a USER sends SHALL instead carry that user's answer about a signed-out pane: refuse,
restart the agent and deliver once it is back, or deliver regardless. The reading is an observation
of the agent's screen and may be older than what the user knows, so it SHALL NOT overrule them. The
answer SHALL apply only where the pane actually reads signed out, so one given in advance never
disturbs a session that turns out to be fine.

The restart SHALL be performed by the hub rather than recommended to the user: it replaces the pod
on the same identity, so the session resumes and the process re-reads the credentials the hub keeps
staged. A restart that fails SHALL say that is what failed, rather than reporting a message that
quietly went nowhere.

Every interface that sends a message SHALL offer both answers, and any refusal it shows SHALL name
what the user can do from where they are.

#### Scenario: A hub message finds a signed-out agent

- **WHEN** the hub injects a verdict or an assignment into a signed-out session
- **THEN** delivery fails and the attempt is recorded, rather than the message vanishing into an
  input box

#### Scenario: The user restarts and sends

- **WHEN** a user tells a signed-out agent and asks for the restart
- **THEN** the agent is restarted on the same identity and the message is delivered once its
  session is back

#### Scenario: The user knows better than the pane

- **WHEN** a user tells a signed-out agent and asks to send regardless
- **THEN** the message is delivered without further objection

#### Scenario: An answer given in advance about a healthy agent

- **WHEN** a message carries an answer about a signed-out pane, and the agent reads fine
- **THEN** it is delivered as any other message, and the agent is not restarted

#### Scenario: A refusal names what can be done

- **WHEN** an interface refuses to send to a signed-out agent
- **THEN** it names both answers in terms the user can act on there and then
