# hub — delta

## ADDED Requirements

### Requirement: Open PRs are kept honest as their base moves

After the reference branch moves, the hub SHALL determine for each open PR whether it would still
apply onto that base, and — where it would — whether the result of combining the two would pass the
project's gate. Neither is known today until a human attempts the merge.

The applies-question SHALL be answered by performing the same operation the merge performs, ONTO THE
SAME BASE. The base is the one recorded on the PR, not the project's current reference: those differ
exactly when the reference is moved or re-pointed, which is the event this check runs on, and a check
against the wrong base reports on an operation nobody will perform — it can pass a PR that conflicts
with the branch it will actually be merged into.

Nor SHALL the check answer from a simulated merge where the hub merges by replaying: a commit whose
content the base already holds conflicts on replay while a merge reports it clean, so the two
questions have different answers and only one of them is being asked.

The combined result SHALL be materialised in a workspace reserved for the purpose, and the hub SHALL
NOT use the author's own workspace for it. Replaying the base where an agent may still be working
moves the ground under it, and merging the base into the branch to inspect it leaves a merge commit
that makes the eventual replay harder — the failure the check exists to find, caused by finding it.

The finding SHALL be recorded against the PR for a human to read, and SHALL NOT change the PR's
status or route it back to its author. A finding SHALL carry its evidence: the conflicting paths, or
the gate's own output. A check that cannot be performed SHALL be recorded as a check that did not
run, never as a finding about the PR.

The work SHALL be bounded: only PRs the base has moved past, one at a time, and a state already
answered SHALL NOT be answered again until something moves — where "something" includes a PR being
aimed at a different base, which is a new question even when no tip has changed. A full gate run per
open PR per sweep would leave the host permanently busy.

The check SHALL NOT hold up the detection of reference drift. Its gate run can take minutes, and the
loop that notices a branch has moved SHALL keep its own cadence rather than waiting behind it.

#### Scenario: A PR level with its base

- **WHEN** the reference branch moves but a given open PR is not behind it
- **THEN** no combined check is performed for that PR

#### Scenario: A PR that no longer applies

- **WHEN** an open PR would conflict when replayed onto the moved base
- **THEN** the PR carries a finding naming the conflicting paths, and its status is unchanged

#### Scenario: A PR that applies but breaks the gate

- **WHEN** an open PR replays cleanly onto the moved base and the combined result fails the gate
- **THEN** the PR carries a finding quoting the gate's output

#### Scenario: A PR whose base is not the project's reference

- **WHEN** an open PR is recorded against a base other than the project's current reference
- **THEN** it is checked against, and its finding names, the base recorded on the PR

#### Scenario: A commit the base already contains

- **WHEN** an open PR contains a commit whose content the base has since acquired
- **THEN** the check reaches the same verdict the hub's own merge would, rather than reporting a
  conflict the merge would not hit or a success it would

#### Scenario: The author's workspace is untouched

- **WHEN** the hub checks what an open PR would combine to
- **THEN** the author's workspace and branch are left exactly as they were

#### Scenario: A burst of base movement

- **WHEN** the base moves several times in quick succession
- **THEN** each open PR is checked against the resulting state rather than once per movement

#### Scenario: A PR aimed at a different base

- **WHEN** an open PR's recorded base is changed to another branch
- **THEN** it is checked again, and the finding names the new base

#### Scenario: Drift detection continues during a check

- **WHEN** a combined check is running for one project
- **THEN** reference drift is still detected on its own cadence, for that project and the others

#### Scenario: The check cannot run

- **WHEN** the combined result cannot be materialised
- **THEN** that is recorded as a check that did not run, and the PR carries no finding about its
  own state
