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

### Requirement: The room says nothing while it is locked

Nothing about the meeting SHALL reach an agent while the room is locked. A locked room
can neither send nor receive, so a message about membership there interrupts with no
action behind it — and invites the agent to speak into a room that will refuse it. This
covers the membership cue a relaunched member is given: it is delivered only while the
room is open, since relaunches happen on a schedule the user did not choose and
membership outlives the meeting.

A user ACTING on the room SHALL count as presence — adding or removing a member,
closing it, saying something. The lock asks whether a human is at the room, and one who
has just acted on it is; without that, a membership change made from an interface that
sends no heartbeat would land in a room read as empty, and the agent would never learn
it had been added.

The notice that membership has ENDED is exempt: it prevents a useless action rather
than inviting one, and an agent that believes it is still in a closed room will try to
speak into it.

#### Scenario: A member is relaunched while the room is dormant

- **WHEN** an agent that belongs to a room nobody has opened is relaunched
- **THEN** it is told nothing about the meeting

#### Scenario: A member is relaunched during a meeting

- **WHEN** an agent that belongs to an open room is relaunched
- **THEN** it is reminded that it is a member and how to speak

#### Scenario: A member is added from an interface that does not heartbeat

- **WHEN** the user adds an agent from the CLI
- **THEN** the room counts the user as present and the newcomer is welcomed and caught up

#### Scenario: Membership ending is always delivered

- **WHEN** a room closes while nobody is present
- **THEN** its members are still told they are no longer in it
