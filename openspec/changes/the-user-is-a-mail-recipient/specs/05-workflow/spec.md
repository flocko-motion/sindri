# Workflow — delta

## ADDED Requirements

### Requirement: An agent may write to the user, against a budget

The user SHALL be a mail recipient like any agent, addressed by the same name that stamps a
message from them, and no agent SHALL be permitted to take that name — one mailbox with
both in it, and nothing in a row to tell them apart, is the collision that reservation
prevents.

An agent SHALL have a verb for one short note to the user about something it noticed IN
PASSING. The register SHALL be stated wherever the verb appears — in the agent's brief, in
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
same grouping, so each answers "is anything waiting for me?" the same way.

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
