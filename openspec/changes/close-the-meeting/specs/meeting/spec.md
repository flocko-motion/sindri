# meeting — delta

## ADDED Requirements

### Requirement: A meeting can be closed, and closes itself when it is over

Closing a meeting SHALL remove every member and tell each of them, so nothing goes on
believing it is in a room that no longer takes what it says. It SHALL leave the
transcript alone: clearing the history is a different act, and a closed meeting is
still worth reading. Closing an empty room SHALL do nothing at all — no notice, no
transcript line — so the action is safe to repeat.

Membership is durable, and until now only an explicit removal ever ended it, so a room
the user had not opened for days still had members and those members were still spoken
to. Closing is what accepts the cost `new meeting` deliberately refuses: re-adding
members is manual work, so an interactive front-end SHALL confirm first, while the CLI
runs immediately as its other destructive verbs do.

A room in which nothing has been said for an hour SHALL close itself the same way. The
hour is measured from the last MESSAGE rather than from the presence lock: the
transcript is the record of the meeting, and an interface left open on the room would
otherwise keep an empty meeting alive precisely when nobody is holding it.

An automatic close SHALL NOT interrupt anybody. Its notices SHALL be delivered when an
agent is next at an idle prompt — a deliberate close has just been asked for and may
interrupt, but nobody is worth waking to be told that a meeting they had forgotten is
over. Both SHALL leave a trace: the reason is recorded in the transcript, so whoever
opens the room next can see why it is empty, and against each agent as membership
changes already are.

#### Scenario: The user closes the meeting

- **WHEN** the user closes a room with members in it
- **THEN** every member is removed and told, the transcript is kept, and the reason is
  recorded in it

#### Scenario: Closing an empty room

- **WHEN** the user closes a room nobody is in
- **THEN** nothing happens: no notice, no transcript line

#### Scenario: A meeting nobody has held for an hour

- **WHEN** an hour passes with nothing said in a room that has members
- **THEN** the room closes itself, and the transcript says why it emptied

#### Scenario: The automatic close does not wake anyone

- **WHEN** a room closes itself
- **THEN** each member's notice waits for an idle prompt rather than interrupting the
  turn it is running

#### Scenario: A room still in use stays open

- **WHEN** the room has been spoken in within the hour
- **THEN** it stays open, whatever the presence lock reads
