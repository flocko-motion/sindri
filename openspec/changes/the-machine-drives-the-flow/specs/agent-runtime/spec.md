## ADDED Requirements

### Requirement: The pod's lifecycle is the flow machine's

Starting and stopping an agent's pod SHALL be transitions the flow machine performs,
declared as states in every role's map. A human's request to start or stop an agent
SHALL be recorded on the roster row and announced, and the machine SHALL carry it
out.

No sweep SHALL start or stop a pod. The rules deciding whether an agent may be woken
or reclaimed stay where they already live, in the surface derived from an agent's
situation; the machine is what acts on them.

#### Scenario: Work arriving for an empty pool starts an agent

- **WHEN** work is waiting in a repository and every agent of the role that would
  take it has been reclaimed for idleness
- **THEN** the machine starts one of them, and the work stays waiting until it is up

#### Scenario: A claim never outlives the pod it was made for

- **WHEN** an agent is handed work
- **THEN** its pod is up, or the machine is standing in the state that starts it,
  so no claim can erase the evidence that an agent was needed

#### Scenario: Reclaiming an idle pod is a transition

- **WHEN** an agent has held nothing past the idle threshold
- **THEN** the machine moves it to its stopping state, and the coauthor is exempt
  because the surface refuses to reclaim a session the user is sitting in

### Requirement: Every role carries the lifecycle states

Every role's map SHALL declare the full session and pod lifecycle — launching,
stopping, clearing, mail, escalated and retired — however few work states that role
has. A role the machine cannot reach for a clear or a launch SHALL NOT exist.

These states SHALL be built by a factory taking each role's own destinations, so
that the set of events a lifecycle state handles is written once and a role omitting
one fails to compile. Where a lifecycle state leads differs by role; what can move a
subject out of it does not.

#### Scenario: A thin role is still driven by the machine

- **WHEN** a human arms a context clear for a coauthor or a planner
- **THEN** the machine fires it from that role's own clearing state, with no sweep
  standing in for the roles its maps do not reach

#### Scenario: A missing lifecycle edge fails the build

- **WHEN** a role's map declares a lifecycle state without a destination for one of
  that state's events
- **THEN** compilation fails, because the destination is a required field of the
  factory's argument rather than a line that can be left out
