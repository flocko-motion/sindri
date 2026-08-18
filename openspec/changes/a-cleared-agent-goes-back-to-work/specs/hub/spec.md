# hub — delta

## ADDED Requirements

### Requirement: A measurement that an action invalidates is not served after it

A cached measurement SHALL be discarded by the action that invalidates it, before anything is
decided from it — where the hub caches a reading of an agent and then performs the very action that
changes it.

Clearing an agent's context is the case: the size of a session is cached so the frequent idle poll
does not re-read a transcript per request, and the clear is followed by asking the agent to request
work. Serving the pre-clear size there tells the agent it is still too full to work — the exact
remedy that has just been applied — and it is deterministic rather than a race, since the ask lands
well inside the cache window.

The invalidation SHALL be at the action rather than in a shorter cache lifetime. The lifetime is
right for every other reader; what was wrong is that one reading outlived the thing it measured.

Every reader of that measurement SHALL see the correction, not only the one that prompted it: a
board that still showed the agent as full would contradict the work it had just been handed.

#### Scenario: An agent is put back to work by clearing it

- **WHEN** an agent retired for a full context is cleared and then asks for work
- **THEN** it is handed work, and is not told it is still full

#### Scenario: The board agrees

- **WHEN** an agent's context is cleared
- **THEN** its status stops reporting it as full at the same moment

### Requirement: An agent parked by the hub is not chased for being idle

The hub SHALL NOT nudge an agent for being idle when it has told that agent to stop asking for work
and wait — retired by a human, or retired by its own context filling. The agent is idle by
instruction, and prodding it complains about the one state the hub deliberately put it in, asking it
to do the thing it was just told to stop doing.

This applies to the idle nudge alone. A turn cut off mid-sentence SHALL still be asked to resume,
whatever the agent's standing: that is not a complaint about idling, and being wound down is no
reason to leave the last turn broken.

#### Scenario: A retired agent waits as instructed

- **WHEN** an agent the hub has retired sits idle holding work
- **THEN** it receives no stall nudge

#### Scenario: A genuinely stalled agent is still nudged

- **WHEN** an agent nobody parked goes quiet holding work
- **THEN** it is nudged as before

#### Scenario: A cut-off turn is retried regardless

- **WHEN** a parked agent's turn is cut off by an API error
- **THEN** it is still asked to resume
