# hub — delta

## ADDED Requirements

### Requirement: An agent undoes its own work by rolling back, not by restoring

The curated git surface SHALL offer an agent a rollback to a point in its OWN history rather than a
restore of uncommitted changes, because the hub records the workspace as a commit every time it gates
it: there are never unrecorded changes left to put back. Asking for the withdrawn verb SHALL be
answered by saying so, rather than accepted as a no-op that reads like a rescue.

The point named SHALL be one the agent could have read from its own history listing: it SHALL resolve
to a commit, SHALL be in the agent's current branch's history, and SHALL NOT be earlier than where
that branch left the reference branch. A rollback SHALL discard everything after it, including work
never handed over, and SHALL say what it discarded — the reset leaves no other trace an agent can
read. A refused rollback SHALL move nothing and SHALL name what to do instead.

This is the one place an agent names something other than a path, so the id is validated by the hub
rather than passed to git: a branch or tag name is not this verb's business.

#### Scenario: Rolling back to an earlier point of its own

- **GIVEN** an agent whose history has a point before its latest work
- **WHEN** it rolls back to that point
- **THEN** its workspace matches that point, everything after it is gone — uncommitted edits
  included — and the reply says what went

#### Scenario: An id that is not the agent's own is refused

- **WHEN** an agent names a commit that is not in its branch's history, or one older than where its
  branch left the reference branch
- **THEN** the rollback is refused, nothing moves, and the reply points at the agent's own history

#### Scenario: The withdrawn verb explains itself

- **WHEN** an agent asks to restore paths
- **THEN** it is told the verb is gone, why there is nothing left for it to put back, and which two
  verbs cover what it wanted
