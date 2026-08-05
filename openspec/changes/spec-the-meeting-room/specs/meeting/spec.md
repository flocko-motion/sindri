# Meeting room — delta

## ADDED Requirements

### Requirement: The room is a hub-owned star, never a mesh

The meeting room SHALL be a shared conversation between the user and a selected set of
agents, with the hub as its centre: every message SHALL be recorded by the hub and
forwarded by the hub to each member except its sender. An agent SHALL NOT address the
room's other members directly, and SHALL NOT learn who they are except through what the
hub forwards — consistent with agents having no roster visibility and no peer addressing.

Membership MAY span projects, because an agent's name is globally unique and the room is
the user's, not a repository's. An agent's own isolation SHALL be unaffected: being in
the room SHALL NOT give it visibility of another project's tasks, PRs or agents.

#### Scenario: A message reaches every other member

- **WHEN** a member sends a message to the room
- **THEN** the hub records it and forwards it to every other member, and not back to the
  sender

#### Scenario: Agents in one room, still isolated

- **WHEN** agents from two different projects are members of the same room
- **THEN** each sees only what the hub forwards, and neither gains access to the other's
  project

### Requirement: The user curates membership; agents cannot

Adding and removing members SHALL be the user's action alone, available as an explicit
command and as an in-room command. An agent SHALL NOT be able to change membership —
neither its own nor another's — even though it can speak in the room.

Adding an agent already present SHALL be a no-op rather than an error. Removing an agent
SHALL tell that agent it has been removed. Deleting an agent SHALL drop its membership,
so the roster never names an agent that no longer exists.

#### Scenario: Agent cannot add or remove

- **WHEN** an agent attempts to change the room's membership
- **THEN** it cannot: only the user's path interprets membership commands

#### Scenario: Adding twice changes nothing

- **WHEN** the user adds an agent that is already a member
- **THEN** the roster is unchanged and no notice is sent

#### Scenario: A removed agent is told

- **WHEN** the user removes an agent from the room
- **THEN** that agent is informed it has been removed

#### Scenario: Deleting an agent leaves the room

- **WHEN** an agent that is a member is deleted
- **THEN** its membership goes with it

### Requirement: The user is a required participant

The room SHALL be open only while the user is present: presence is recorded when the
user is in the room, and expires after a bounded interval without a sign of them. While
the user is absent the room SHALL be closed, so agents cannot hold a conversation among
themselves with nobody leading it.

#### Scenario: Absent user closes the room

- **WHEN** no sign of the user has been recorded within the presence interval
- **THEN** the room is closed and an agent's attempt to use it is refused

#### Scenario: The user's return opens it

- **WHEN** the user is present again
- **THEN** the room is open and members can speak

### Requirement: A newcomer is caught up in one bounded delivery

An agent joining a room with history SHALL receive that history as a **single**
delivery, because a delivery is typed into the agent's session and embedded newlines
would submit each line as a separate prompt. The catch-up SHALL be bounded by the same
limit a single message has, and when it must drop content it SHALL drop the **oldest**
first — what was said most recently is what the newcomer is about to be asked about.

#### Scenario: History arrives as one line

- **WHEN** an agent is added to a room that already has a transcript
- **THEN** it receives the history as one delivery, not one per message

#### Scenario: Over-long history drops the oldest

- **WHEN** the history exceeds the delivery limit
- **THEN** the oldest messages are dropped and the most recent are kept

### Requirement: A new meeting clears the history and keeps the roster

Starting a new meeting SHALL clear the shared transcript and announce the fresh start to
the room, and SHALL leave membership intact — a new meeting is about the history everyone
shares, not about who is in the room. Because clearing cannot be undone, the interface
SHALL confirm before it happens, and SHALL report whether there was anything to clear.

#### Scenario: History cleared, members kept

- **WHEN** the user starts a new meeting
- **THEN** the transcript is emptied, the room is told, and every member is still a
  member

#### Scenario: Confirmed before clearing

- **WHEN** the user triggers a new meeting from either front-end
- **THEN** the action is confirmed first, since it is irreversible

### Requirement: In-room commands and hub replies belong to the user

The room SHALL accept in-room commands from the user — at least adding and removing a
member, listing the roster, and help — distinguished from ordinary messages by a leading
marker. A front-end SHALL determine whether a line is a command by asking the shared
rule rather than reimplementing it, so a composer and the hub never disagree about what
was typed.

A hub reply to such a command SHALL be recorded for the user only and SHALL NOT be
forwarded to the members, so the room's transcript is a conversation and not an audit of
the user's tooling.

#### Scenario: A command is not broadcast

- **WHEN** the user types an in-room command
- **THEN** the hub acts on it and replies to the user, and no member receives the command
  or the reply

#### Scenario: One rule for what a command is

- **WHEN** a front-end decides whether the composed line is a command or a message
- **THEN** it consults the shared rule, so both front-ends and the hub agree

### Requirement: Delivery is best-effort and never fails a broadcast

Forwarding to a member that is not running, or whose delivery fails, SHALL NOT fail the
broadcast for the others: the message SHALL still be recorded and delivered to every
reachable member, and the failure SHALL be logged rather than surfaced as the sender's
error.

#### Scenario: One member offline

- **WHEN** a message is broadcast and one member's session is not running
- **THEN** the message is recorded and reaches every other member, and the sender sees no
  error

### Requirement: Both front-ends present the same room

Every action the room offers SHALL be reachable from both front-ends: adding and removing
members, speaking, reading the transcript, and starting a new meeting. The room SHALL
present identically in both — the same participant markers and the same help text, drawn
from the shared rendering module so neither front-end invents its own.

#### Scenario: Membership from either front-end

- **WHEN** the user adds or removes a member
- **THEN** the action is available in both the CLI and the TUI, not one of them only

#### Scenario: The same room, twice

- **WHEN** the same room is shown in the CLI and in the TUI
- **THEN** participants carry the same markers and the help text is the same, because
  both take them from the shared rendering module
