# Hub routing — delta

## MODIFIED Requirements

### Requirement: One delivery primitive, hub-owned routing

All inbound delivery to an agent SHALL go through the hub, which SHALL be the sole
holder of the `(project, name)→pod` and `(project, object)→pod` routing tables,
derived from its central store. No other actor SHALL resolve an agent to its pod, and
resolution SHALL be scoped to the agent's project.

Delivery SHALL offer TWO paths, and every sender SHALL state both of the independent
properties that choose between them:

- **must the agent READ it?** — if so the message is MAILED: kept until the agent
  reads it, whatever it was doing when the message was sent.
- **should it ACT NOW?** — if so the message is PUSHED: typed into the agent's live
  session, which wakes it and is best-effort by nature.

A message MAY be both, and the common case is: a verdict must not be missed AND
should be acted on now. A message that is neither is not a message.

Push SHALL remain best-effort, and that is not a defect of it: an agent that is down,
restarting, mid-clear or blocked at a prompt cannot be typed into, and for traffic
whose whole value is waking a live agent there is nothing worth keeping. What SHALL
NOT happen is a message that must be read taking that path alone, which is silent
loss with no record that anything was missed.

Mail SHALL be durable, and the hub SHALL write it BEFORE attempting the push, so a
failure between the two loses only the wake. A mail record SHALL say whether its push
actually landed, recorded from the delivery's outcome and never from the sender's
intent — "it may have acted on this already" and "nothing has reached it" are
different diagnoses and a reader needs to tell them apart.

#### Scenario: Hub routes by (project, name)

- **WHEN** a message is addressed to an agent in a given project
- **THEN** the hub resolves it within that project to the agent's pod and delivers
  the message by the paths its sender declared

#### Scenario: A message that must be read reaches an agent that was away

- **WHEN** a message declared as mail is sent to an agent whose session cannot be
  typed into
- **THEN** it is kept until that agent reads it, and is recorded as not pushed

#### Scenario: A wake nobody received is not kept

- **WHEN** a message declared push-only cannot be delivered
- **THEN** nothing is stored, because being live was the whole of its value

### Requirement: The user chooses the path, as two actions

A user SHALL be able to PUSH a message to an agent or MAIL one, as two separate actions
in both front-ends, exposing the same two questions every sender inside the hub answers.

Push (`tell`) SHALL keep its meaning exactly: it types into the live session now, so it
interrupts, and it is lost if the agent is not there. That is the right verb for "stop
what you are doing", where a message that misses an absent agent is moot anyway, because
the situation will have changed by the time it returns.

Mail SHALL wait to be read and SHALL NOT interrupt — and SHALL NOT push either, even
when the agent is reachable. Choosing mail over push IS the choice not to interrupt, so
notifying anyway would defeat the only reason to pick it. This is where a mailed message
differs from a verdict, which must be read AND acted on now.

Because mail never touches the session, it SHALL reach an agent that push cannot: one
that is down, restarting, mid-clear, or signed out. The refusal that protects a
signed-out pane belongs to the PUSH path alone — text typed at a login prompt vanishes
unread, which is a fact about typing, not about the message.

The two SHALL NOT be folded into one action. The difference SHALL be legible where the
choice is made — in the verb and in its one line of help — rather than in documentation
the user will not read: one interrupts and may be lost, the other waits and will be read.

#### Scenario: Mailing an agent that is away

- **WHEN** the user mails an agent that is down, restarting or signed out
- **THEN** the message is kept and is read when that agent next asks the hub what to do

#### Scenario: Mail does not interrupt

- **WHEN** the user mails an agent that is running and mid-turn
- **THEN** nothing is typed into its session, and it reads the message at its next ask

#### Scenario: Push keeps its meaning

- **WHEN** the user pushes a message to an agent that is not there to receive it
- **THEN** it is reported as undelivered rather than kept, because waking was the point

#### Scenario: A sender that declares neither

- **WHEN** a delivery asks for neither mail nor push
- **THEN** it is refused as a fault rather than silently doing nothing
