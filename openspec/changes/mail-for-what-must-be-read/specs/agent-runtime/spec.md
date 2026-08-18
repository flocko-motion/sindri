# Agent runtime — delta

## MODIFIED Requirements

### Requirement: No blocking — act, report, then idle

Every hub call SHALL return promptly. After reporting (for example, registering a
branch for merge), the agent SHALL go idle at its prompt rather than hold a call
open. The agent SHALL be instructed that waiting is expected and may take a long time.

An idle agent SHALL be resumed either by the hub PUSHING a message into its session,
or — for anything it must not miss — by MAIL it collects on its own next ask. Waking
by injection alone was the whole mechanism, and it cannot reach an agent that is down,
restarting, mid-clear or blocked at a prompt; such a message was lost with no record
of it. So the hub's answer to an agent asking for its next action SHALL tell it when
mail is waiting, and an agent SHALL be able to collect that mail explicitly.

Collecting SHALL be explicit and SHALL mark rather than consume: a message stays on
the record once read, and an agent that crashes between delivery and processing finds
its mail still waiting — which is the guarantee an injection cannot give.

Mail SHALL NOT expire. A message is kept precisely because it must be read, and no
clock can tell whether it is still true, so RELEVANCE IS SETTLED AT PICKUP: a mailed
message points at state the agent can re-read, and the reader checks it against that
state rather than trusting the message's age. Where the two disagree, the live state
wins.

#### Scenario: Submit returns immediately

- **WHEN** an agent registers its branch for merge
- **THEN** the call returns at once ("registered, please wait") and the agent goes
  idle instead of blocking

#### Scenario: Woken by injection

- **WHEN** the hub has the next task or a verdict
- **THEN** it injects the message into the idle agent's session to resume it

#### Scenario: Told about mail when it asks

- **WHEN** an agent asks for its next action and has unread mail
- **THEN** it is told how much is waiting and how to read it, ahead of any other
  directive, since a verdict or a cancellation can change what that action is

#### Scenario: An agent that was told to sit still is told too

- **WHEN** an agent stopped on a decision only the user can make asks for its next
  action and has unread mail
- **THEN** it is told about the mail first, because it is the state least likely to
  find out by chance and the mail may answer or moot its question — and any claim the
  hub makes that nothing has arrived is made only once the mailbox is empty

#### Scenario: Reading is explicit and keeps the record

- **WHEN** an agent reads its mail
- **THEN** each message is handed over and marked read, and remains readable
  afterwards by a human

#### Scenario: An older message is checked against the live state

- **WHEN** an agent reads a message about work that has since moved on
- **THEN** it is told to check each message against the current state before acting,
  and the live state decides
