# view-workers — delta

## ADDED Requirements

### Requirement: Agents that need the user are marked

An agent SHALL count as needing the user when its state RESOLVES ONLY IF A HUMAN
ACTS. That is the rule; the states satisfying it today are:

- **blocked** — stopped at a prompt waiting for an answer, so it asks and then holds
- **signed-out** — its pane says to run `/login`, so nothing typed there is sent
- **full** — past its context window with nothing in hand, handed no work until cleared
- **stalled** — holding work with its screen standing still, believing it is working

A status added later belongs to this set when the same question answers yes of it.

Idle SHALL NOT count. Idle splits three ways and each has one correct response: idle
with no work available is healthy and is left alone; idle beside work it could claim
is a dispatch fault the hub nudges; idle because a human must act is this set, which
is marked and never nudged, since prodding an agent complains about a state the hub
itself put it in. Marking a plain idle agent would make the marker mean "an agent
exists", which the roster count already says.

A retired agent SHALL NOT count: the user has taken that decision already. An agent
whose turn was cut off by an API error SHALL NOT count while the hub is resending it;
once resending stops working its screen stands still and it is stalled, which counts.

Every workers view SHALL make this visible. The CLI listing SHALL mark each such row
and SHALL close by naming those agents and what clears them, because a status column
is skimmed and each of these agents otherwise reads as one at work.

#### Scenario: An agent stopped at a prompt

- **WHEN** an agent is blocked, signed out, full or stalled
- **THEN** the workers view marks it as waiting on the user, and the CLI listing names
  it in a closing line with the action that clears it

#### Scenario: An idle agent is not a fault

- **WHEN** an agent is idle, whether or not work is available
- **THEN** no marker is shown for it, and nothing asks the user to act

#### Scenario: A retired agent asks nothing further

- **WHEN** an agent has been retired by the user
- **THEN** it is not marked as waiting on them
