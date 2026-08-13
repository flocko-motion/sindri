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

#### Scenario: A sender that declares neither

- **WHEN** a delivery asks for neither mail nor push
- **THEN** it is refused as a fault rather than silently doing nothing
