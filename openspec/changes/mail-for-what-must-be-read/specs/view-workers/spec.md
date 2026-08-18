# view-workers — delta

## ADDED Requirements

### Requirement: Mail is a view of its own, and an agent's backlog shows on its row

Every message an agent must read SHALL be visible to the user in a view of its own,
spanning agents and repos rather than one timeline per agent — "what has this agent
been told?" is asked of a fleet, and the answer is a fleet-wide record.

A row SHALL show the recipient and its repo, what sent the message, when it was sent,
whether and when it was read, and an opening of the message. The FULL body SHALL be
readable on request rather than in the list: a rejection carries its whole findings and
can run to hundreds of lines. Each row SHALL also say whether the message was pushed,
because "pushed and possibly missed" and "sitting here unread" are different
diagnoses.

Both front-ends SHALL offer the view, through the section model that decides which
views exist, so neither can drift from the other. Its badge SHALL be UNREAD mail: the
mailbox is never pruned, so a total would only ever climb, while unread can return to
zero. It SHALL NOT claim the user's attention marker — unread mail is the AGENT's
backlog, and no verb of the user's clears it.

Both front-ends SHALL narrow the view the same way, by unread-or-all and by one
recipient, from one shared definition of those filters.

Where the view shows a WINDOW of the mailbox rather than all of it, it SHALL say so
and say how much it is not showing, and older mail SHALL stay reachable by id. A
history kept for ever and then quietly cropped is worse than one that admits its
window, because being able to find last month's message is the reason for keeping it.

An agent's unread count SHALL appear on the agent itself, in both front-ends. A
mailbox waits quietly by design, so a backlog is the only signal that an agent has
stopped reading, and nothing else surfaces it.

#### Scenario: Reading what an agent was told

- **WHEN** the user opens the mail view
- **THEN** every message sent to any agent is listed with its recipient, sender, time
  and read state, newest first

#### Scenario: A long message

- **WHEN** the user opens a message whose body is long
- **THEN** the list showed an opening marked as cut, and the full text is shown on
  request

#### Scenario: A windowed listing

- **WHEN** the mailbox holds more messages than the view carries
- **THEN** the view states how many of how many it is showing, and how to reach the
  rest

#### Scenario: An agent that has stopped reading

- **WHEN** an agent has unread mail
- **THEN** its count shows on that agent in both front-ends
