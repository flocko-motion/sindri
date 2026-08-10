# Hub — delta

## MODIFIED Requirements

### Requirement: Orphans are runtime the roster does not account for

The roster in `hub.db` SHALL be the declaration of which agents exist; reality SHALL
be checked against it, not the other way round. A pod or worktree running with no
matching roster entry SHALL be reported as an orphan. The hub SHALL NOT kill an orphan
on its own initiative — no sweep, no reaping, nothing dies unasked. It SHALL surface the
orphan as a warning and SHALL offer removal as an explicit user-initiated action, which
every front end can invoke; the front end SHALL confirm before it is carried out. The
mechanism is the hub's own, not a container-engine command the user is asked to run.

#### Scenario: Orphan detected

- **WHEN** a pod is running with no matching roster entry
- **THEN** it is reported as an orphan the user may remove, and nothing is killed
  automatically

#### Scenario: Orphan removed on request

- **WHEN** a user confirms removal of a reported orphan from either front end
- **THEN** the hub removes that runtime, and no roster entry is touched — there was none

#### Scenario: Declared agent with no pod is not an orphan

- **WHEN** an agent is in the roster but has no running pod
- **THEN** it is a stopped, launchable agent — not an orphan
