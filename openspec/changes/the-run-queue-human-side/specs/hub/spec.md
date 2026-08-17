# hub — delta

## ADDED Requirements

### Requirement: The user can queue a run, into the same slot agents share

A user SHALL be able to queue a run from the CLI and from the TUI. It SHALL join the same queue an
agent's run joins — one run at a time across the whole fleet — because a human who cannot queue one
either asks an agent to do it or runs it outside the queue, and running it outside is exactly the
uncoordinated concurrency the queue exists to prevent.

The target SHALL be explicit in the request rather than inferred from a working directory. The same
command means different things in different trees, and a run occupies the fleet's only slot, so a
wrong guess is expensive. The targets are an AGENT'S WORKSPACE, named, and the REPO'S OWN CHECKOUT.

The repo's own checkout is a permitted target, deliberately. A run executes against a COPY of its
target, made when the run reaches the front, so nothing the command writes can reach the tree the
user is working in — the isolation that already exists for agent worktrees answers this case
unchanged. Testing uncommitted work is the point of the target, not a hazard of it.

A user's run SHALL outrank every agent's, gate runs included. Somebody is waiting on it, while the
agent behind an agent run is parked and watching nothing; what the wait costs that agent is bounded
by the run's own cap. The user SHALL still be able to reorder it by hand afterwards — the origin
decides where it starts in the queue, never that it is fixed there.

Everything else SHALL be inherited rather than rebuilt: the cap, the fresh container, the stored
output, cancellation, and the queue position. A user run is a queue entry like any other.

Nothing that treats the scheduler as an AGENT SHALL apply to a user's run — a roster lookup, a
staleness check, a result injected into a session. There is no agent to have moved on and no session
to inject into: the result reaches the user on the board.

Every front-end SHALL show who asked for each run, distinguishing a user's from an agent's, and
SHALL name the target it runs against. A column of agent names with one entry that is not an agent
lies about the run's origin, and a run without its target cannot be interpreted.

#### Scenario: A user queues a run against the repo's checkout

- **WHEN** the user queues a run without naming an agent
- **THEN** it is queued against the repo's own checkout, uncommitted work included, and executes
  against a copy of it

#### Scenario: A user queues a run against an agent's workspace

- **WHEN** the user queues a run naming an agent
- **THEN** it is queued against that agent's workspace, and an unknown name is refused rather than
  run somewhere else

#### Scenario: A user's run goes first

- **WHEN** a user queues a run while agent runs, gate runs among them, are already queued
- **THEN** the user's run is next, and it can still be reprioritised by hand

#### Scenario: A user's run is never dropped as stale

- **WHEN** a user's run reaches the front of the queue
- **THEN** it executes, since it has no scheduling agent that could have gone away or moved on

#### Scenario: Who asked, and what it ran against

- **WHEN** a run is listed or shown, in either front-end
- **THEN** a user's is distinguishable from an agent's, and the workspace it runs against is named
