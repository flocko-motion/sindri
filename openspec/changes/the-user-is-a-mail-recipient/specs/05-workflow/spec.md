# Workflow — delta

## ADDED Requirements

### Requirement: A message carries the sender its sender states

Every message SHALL record WHO it is from, stated by whoever sends it. It SHALL NOT be
inferred from the message's text: reading provenance out of a prefix in the body makes it a
property of how a message happens to be worded, and under that scheme only two of the four
senders the record documents — the hub, the user, a reviewer, an agent by name — could ever
be stored at all.

The sender SHALL travel with the delivery classification, as one statement of how a message
travels and who it is from, so a call site cannot state one and forget the other. An
unstated sender SHALL mean the hub speaking in its own voice, which is what an
unattributed hub-originated message IS rather than a value to be guessed at. A provenance
tag MAY remain in the rendered text where a reader wants it, but it SHALL NOT be the
mechanism.

There SHALL be ONE delivery path. A second way to write a message — a sender reaching the
mailbox directly because the shared path could not carry what it needed — is how the two
come to disagree about what a sender is.

#### Scenario: A verdict from a reviewer

- **WHEN** a reviewer's rejection is delivered to the agent that submitted it
- **THEN** the message records that reviewer as its sender, whatever its text says

#### Scenario: A body that quotes a tag

- **WHEN** a message from an agent contains a provenance tag in its own text
- **THEN** the recorded sender is the agent, not the tag

#### Scenario: The user has no session

- **WHEN** a message is delivered to the user
- **THEN** no push is attempted, because their mailbox is the channel — not a delivery that
  failed but one that does not exist

### Requirement: An agent may write to the user, against a budget

The user SHALL be a mail recipient like any agent, addressed by the same name that stamps a
message from them, and no agent SHALL be permitted to take that name — one mailbox with
both in it, and nothing in a row to tell them apart, is the collision that reservation
prevents.

An agent SHALL have a verb for one short note to the user about something it noticed IN
PASSING. It SHALL be offered to exactly the roles that can USE it: a role whose work
grants no budget SHALL NOT see it at all, since a verb advertised and permanently
refused has to invent a reason, and the reason would be false. A coauthor is excluded on
its own grounds — it works beside the user in a shared terminal and already has their
attention, so a note would be a slower way to say what it can simply say. The register SHALL be stated wherever the verb appears — in the agent's brief, in
its help, and in every refusal — and SHALL lead with what the note is NOT, because the
failure mode is over-sending:

- not "I finished X", which the pull request says
- not a summary of the work, which the pull request and the log say
- not a question or a blocker, which is an escalation
- not something about the task in hand, which is a task comment
- but something with no home on this task or pull request, that nobody will learn if this
  agent stays quiet

The test SHALL be given in one line: if you say nothing, does this fact disappear?

The channel SHALL be bounded before it is opened, by numbers that are named constants in one
place, because the first values will be wrong and tuning must not be a search:

- A LENGTH cap. An over-length note SHALL be REFUSED, never truncated: a silent cut drops
  the point and teaches nothing, so the next note is exactly as long. The refusal SHALL say
  to cut it, and SHALL explicitly forbid splitting it across two calls.
- A GRANT PER CLAIM, given whole however long the claim runs, and WORK-BASED rather than
  time-based: time accrues while an agent sits idle, which rewards having seen nothing and
  lets a long-blocked agent wake with a full purse. The grant SHALL REPLACE rather than
  accumulate — banking is what turns a quota into an occasional flood — and an agent that
  has claimed nothing SHALL have nothing granted, so the budget fails closed.
  EVERY role that sees the verb SHALL be granted at its own equivalent of a claim: a task
  for a worker, a review for a reviewer, a brief for a planner. A reviewer reads whole
  diffs across subsystems that are nobody's task, which makes it the role most likely to
  notice something with no other home — so granting only on a backlog task would withhold
  the channel from where it is most useful.
  The refusal SHALL be worded so that it is true both of a grant that was spent and of one
  never given, and SHALL take its number from the constant, so tuning the grant cannot
  leave the sentence describing the old value.
- A FLEET-WIDE CEILING over a rolling window, on top of the per-agent grant. This is the
  limit that protects the user: a work-based budget scales with the fleet and one person's
  attention does not, so a fleet of impeccably behaved agents still buries them, every
  individual decision along the way correct.

At the fleet ceiling the note SHALL be REFUSED rather than held. Refusing is honest and
leaves no queue the user cannot see, where holding preserves a note at the cost of delivering
it hours stale. Because that cost is real — a valuable note can die because two chatty agents
got there first — every refusal SHALL be recorded, since those counts are the only evidence
for what the ceiling should be.

Every send SHALL tell the agent what it has left, and the verb's help SHALL state it too:
known scarcity induces far more selectivity than a cap discovered by hitting it, which makes
the hard limit a backstop rather than the mechanism.

#### Scenario: A note with no other home

- **WHEN** an agent notices something that belongs to no task or pull request and sends it
- **THEN** it is delivered to the user's mailbox, attributed to that agent, and the agent is
  told how much of its grant remains

#### Scenario: An over-length note

- **WHEN** an agent sends a note longer than the cap
- **THEN** it is refused with nothing stored, the agent is told to cut it rather than split
  it, and its grant is not spent

#### Scenario: The grant is spent

- **WHEN** an agent has used its notes for the current claim and sends another
- **THEN** it is refused, and the refusal names where the material belongs instead

#### Scenario: A new claim does not bank the last one

- **WHEN** an agent finishes a claim with notes unspent and claims again
- **THEN** it has exactly the grant, not the grant plus what was left

#### Scenario: The fleet reaches the ceiling

- **WHEN** the fleet has sent the user its hourly ceiling and another agent sends a note
- **THEN** it is refused rather than queued, the refusal says so plainly, and the refusal is
  recorded

#### Scenario: An agent that has claimed nothing

- **WHEN** an agent that holds no claim sends a note
- **THEN** it is refused, because the right to speak follows having been somewhere and looked

### Requirement: The user can see what was said to them

Unread mail addressed to the user SHALL be marked on the mail view's own tab handle, with
the same treatment every other view uses for rows that wait on a person. Without it the
channel is one into a void: an agent's note would sit unread until somebody happened to open
the tab.

The marker SHALL count ONLY user-directed mail. Agent-to-agent and hub-to-agent traffic is
the bulk of the mailbox and none of it is the user's to read, so a marker counting that would
be permanently lit and instantly ignored.

It SHALL count across EVERY repo, whatever repo is in view, and the rows it counts SHALL be
listed whatever repo is in view. A note from an agent in another project is still a note to
the same person, who should not have to tour the repos to learn that someone spoke — and a
marker that pointed at rows the current scope hid would say something waits and then show
nothing.

Those out-of-scope rows SHALL be labelled as coming from elsewhere, under the same heading
every other scoped list uses, because a foreign row that reads as local is worse than one
that is absent. Both front-ends SHALL offer the same narrowing to the user's own mail and the
same grouping, so each answers "is anything waiting for me?" the same way. That narrowing
SHALL NOT depend on a row of the user's already being on screen: it is worth asking
exactly when none is, so it is reachable from any state of the view.

#### Scenario: A note from another repo

- **WHEN** an agent in a repo other than the one in view mails the user
- **THEN** the marker counts it, the row is listed, and it appears under the heading for rows
  from elsewhere rather than among the local ones

#### Scenario: Agent traffic is not the user's business

- **WHEN** the mailbox holds unread hub-to-agent and agent-to-agent messages
- **THEN** the marker counts none of them

#### Scenario: Asking what is waiting

- **WHEN** the user narrows either front-end to their own mail
- **THEN** both show the same set, grouped the same way, and the listing says how many of the
  fleet's unread are theirs

### Requirement: The mail view opens on what is active, not on everything

The mail view SHALL offer an ACTIVE segment — unread, PLUS anything read inside the window
the other views already share — and SHALL open on it. Unread SHALL remain on offer, being
still the sharpest question to ask of a mailbox, but it is not what a view should open on: a
message vanishing from the list as it is read leaves no trace of what was just dealt with.

A message's last change SHALL be when it was READ if it has been, else when it was sent. One
sent days ago and read a moment ago changed a moment ago, and that is what "recently" must
mean for the segment to say anything useful.

The window SHALL be the one the other views use rather than a mail-specific one: if it is
wrong it should be wrong everywhere at once and fixable in one place.

This matters more here than on any other view because no mail is ever deleted: the unbounded
segment grows for the life of the machine, so it is the one view that gets less usable every
day, and a bounded default is what keeps it readable a year on.

The unread MARKER SHALL NOT follow the filter. It says what needs reading, and a message read
ten minutes ago needs nothing.

#### Scenario: Opening the view

- **WHEN** the user opens the mail view without asking for a segment
- **THEN** it shows unread mail plus what was read inside the shared window

#### Scenario: A message read a moment ago

- **WHEN** a message sent days ago is read now
- **THEN** it counts as recently changed and stays on screen, and the marker stops counting it

### Requirement: An agent may mail another agent, anywhere in the fleet

An agent SHALL be able to mail another by BARE NAME, whatever repo either is in. Names are
unique across the fleet by the allocator's convention, and a sender is given a name by
whoever asked it to make contact — so no roster listing accompanies this, and none should:
addressing stays incidental to the instruction that prompted it.

A bare name SHALL be resolved across every project. Ambiguity SHALL be REFUSED rather than
guessed, because mailboxes are keyed by project and name, global uniqueness is only a
convention of the allocator, and delivering to the wrong agent of that name is the failure
worth engineering against. The refusal SHALL name every candidate in full.

A qualified `<repo>/<agent>` form SHALL be accepted and never required. It is what the
refusal points at: a safety check with no way past it would leave an ambiguous recipient
permanently unreachable.

Addressing is bare; ATTRIBUTION IS QUALIFIED. The recipient SHALL see the sender's repo
alongside its name, since a message arriving from another repo says little without it — and
the reply path is then the bare name again.

It SHALL be MAIL ONLY. An agent SHALL NOT be able to push, because waking another agent is
an interruption and agents that can interrupt each other invite a ping-pong nobody asked
for. The recipient reads it at its next ask.

The LENGTH cap SHALL be the one the note channel uses, for the reason that cap exists:
brevity serves whoever must read it, and here the cost is another agent's context. The
per-claim grant and the fleet ceiling SHALL NOT apply — those bound what one person must
read, and this traffic does not reach them.

#### Scenario: Across repos, by bare name

- **WHEN** an agent mails an agent of another repo by bare name
- **THEN** it is delivered to that agent's mailbox, attributed to the sender's repo and name,
  and nothing is pushed

#### Scenario: Two agents of the same name

- **WHEN** a bare name matches an agent in more than one repo
- **THEN** nothing is delivered, and the refusal names both candidates and the qualified form
  that reaches either

#### Scenario: Agent traffic is not budgeted

- **WHEN** an agent has spent its note grant, or has never claimed anything
- **THEN** it may still mail another agent, since that grant bounds the user's attention only

### Requirement: A message can be answered by its id

Anyone holding a message SHALL be able to answer it by its id, and the reply SHALL go to
whoever sent it, resolved from the stored record. That is the point: an agent that has been
mailed can answer without being told a name, so a conversation needs no directory at either
end — and the same is true of the user, reading in either front-end.

A reply SHALL record the message it answers, so an exchange reads as an exchange rather than
as scattered rows the recipient must match up by guesswork.

An agent SHALL be able to answer only its OWN mail. An id is not a licence to read another
agent's mailbox, and a reply to somebody else's message would answer a question its recipient
never saw.

A reply to the HUB SHALL be refused, naming what to use instead. The hub is not a
correspondent: a message from it is a notification, and an answer typed at it would be read
by nobody. Where the answer needs a human, the escalation path is the one that reaches them;
where it belongs to the work, a task comment does.

A reply to the USER SHALL NOT consume the note budget. That budget bounds attention the user
did not ask for, and a reply answers a message they chose to send — charging for it would
penalise answering and teach agents to go quiet when addressed directly. The length cap still
applies, since the cost of a long message is borne by whoever reads it.

Both front-ends SHALL offer the reply, so the user can answer from wherever they are reading.

#### Scenario: Answering without a name

- **WHEN** an agent replies to a message by its id
- **THEN** the reply reaches whoever sent it, recorded as answering that message, and nothing
  is pushed

#### Scenario: Answering a notification

- **WHEN** a reply is attempted to a message the hub sent
- **THEN** it is refused, and the refusal names the paths that do reach a person

#### Scenario: Answering the user costs nothing

- **WHEN** an agent with no note budget left replies to a message from the user
- **THEN** the reply is delivered, because it answers something they chose to send

#### Scenario: Somebody else's mail

- **WHEN** an agent replies to a message addressed to a different agent
- **THEN** it is refused

### Requirement: The hub wakes an agent that has mail waiting

The hub SHALL wake an idle agent that has unread mail. Mail reaches an agent whenever it next
asks the hub what to do, and an idle agent may never ask again — the case already seen, where
an agent finishes its work and simply stops. Without this wake the guarantee that a message
which must be read is kept until it is read would hold of the record and fail in effect,
exactly when it mattered.

The wake SHALL be the hub's, never the sender's. No agent gains the power to interrupt
another: what an agent does is leave mail, and what the hub does is tell an idle agent that
something is waiting — which is already its job.

It SHALL hold for mail from ANY sender — the user, the hub, or another agent — since it is
what makes the distinction between mail and push honest rather than nominal.

An agent that needs a HUMAN SHALL NOT be woken. Blocked, signed out, mid-turn or cut off, it
cannot act on mail, and a nudge it cannot answer is noise on the very signal a user relies on
to notice a stuck agent. Nor SHALL one the hub has parked — retired, or its context full —
since the hub put it there and told it to wait. Nor one holding work, which will ask anyway
and be handed its mail first.

The same waiting message SHALL NOT be nudged for twice. An agent told once and still not
reading is either choosing not to or is wedged, and repeating it every cycle burns its context
and teaches it to skim the one channel it must not skim. Mail that arrives AFTER a wake is a
new thing waiting and SHALL earn another.

#### Scenario: An agent that stopped asking

- **WHEN** an idle agent has unread mail and is sitting at an empty prompt
- **THEN** the hub tells it what is waiting and how to read it

#### Scenario: Told once

- **WHEN** an agent has been woken for the mail it has and has still not read it
- **THEN** it is not woken again for the same message, and is woken again when new mail arrives

#### Scenario: An agent that cannot act

- **WHEN** an agent with unread mail is blocked, signed out, mid-turn, retired or full
- **THEN** it is not woken

### Requirement: A mail id reads as an id

A mail id SHALL be rendered with a prefix, in both front-ends and in every message the hub
writes. Every other id in the system carries one, and a bare integer shown beside agent names,
repo names and ages does not read as something addressable: "reply to 47" is not an
instruction where "reply to ml-47" is.

The stored key SHALL NOT change. Only the rendering does, so nothing migrates and ids stay
globally unique.

BOTH spellings SHALL be accepted wherever an id is read — prefixed and bare. The bare form is
already in users' shell history and in whatever agents have been told, and breaking it to gain
a prefix would be a poor trade. A refusal SHALL show the shape, since somebody who typed a
wrong one has not necessarily seen a right one.

Reading mail SHALL show each message's id. It is the only place an agent learns one, so
without it the reply verb has an argument the agent cannot obtain.

#### Scenario: An id in a row

- **WHEN** a message is listed in either front-end
- **THEN** its id is shown prefixed

#### Scenario: Typing either form

- **WHEN** an id is given prefixed or bare
- **THEN** both resolve to the same message

#### Scenario: Learning an id

- **WHEN** an agent reads its mail
- **THEN** each message shows the id a reply would name
