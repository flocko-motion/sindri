## ADDED Requirements

### Requirement: Only the flow machine acts on an agent

The flow machine SHALL be the only thing that moves an agent, claims work for it,
prepares its session, or starts and stops its pod. Every write that hands an agent
work or takes it away — the agent's flow state, a review's assigned author, a task's
holder, and the closing of a review — SHALL have exactly one caller, which is the
machine's own action or its state writer.

An architecture test SHALL enforce this by call site over the product, with no
allowlist of writers. Prose has already failed to hold this line: the guard on a
retired agent was added to one function and omitted from another three lines away,
and the reviewer's `clearing` state was declared without the session-gone edge its
worker counterpart carries.

What an agent HOLDS and where it STANDS SHALL be written by separate operations, since
they are separate facts with separate writers: a hand-over or a release decides the
first, and the machine alone decides the second.

#### Scenario: A second claimer is refused at build time

- **WHEN** a function outside the machine calls `SetPhase`, `AssignReview`,
  `CloseReviews`, `ClaimLeaf` or `ClaimContainer`
- **THEN** the architecture test fails, naming the call site and the machine action
  that owns that write

#### Scenario: A dead agent is never handed work

- **WHEN** a review is waiting and every reviewer's pod is down
- **THEN** no reviewer is assigned the review, the row stays unclaimed, and the
  machine starts a reviewer before claiming on its behalf

### Requirement: Everything reaching the hub is a signal

Every way the world reaches the hub SHALL be a signal that sets a flag or publishes a
message, and does nothing else — a user action, an API call, a verb an agent types,
and an event from a task source alike. No signal SHALL move a subject, and none SHALL
decide which agent a piece of work goes to. The flow machine SHALL be what changes
state, and it SHALL do so by observing signals against the world they left behind.

A durable signal is a flag or a record the machine may re-read whenever it looks, and
it SHALL carry all of the meaning. A transient signal is a message published on a
topic, and it SHALL carry no payload and no authority.

A request whose meaning is a lasting condition — retire, stop, arm a clear — SHALL be
recorded as a flag on the subject's own row. A request instructing an action where the
resulting state already holds SHALL be recorded with the time it was asked for, so
that "asked for, and not yet done" is a signal the machine can observe.

#### Scenario: A verdict records and announces

- **WHEN** a reviewer approves the pull request it holds
- **THEN** the verdict is written to the review row and the topic is published, and
  the reviewer is moved out of `reviewing` by the machine observing that verdict

#### Scenario: A human retires an agent

- **WHEN** a human retires an agent
- **THEN** the roster row is marked retired and the topic is published, and the
  machine moves the agent to its retired state once it holds nothing

#### Scenario: A human asks for a running agent to restart

- **WHEN** a human asks to restart an agent whose pod is already up
- **THEN** the request is recorded with its timestamp, because no flag on the row
  would distinguish it from the state the agent is already in

### Requirement: An announcement only shortens latency

A published topic SHALL carry no payload and no authority. Every transition a topic
accelerates SHALL also be reachable by the watching state's own poll, so that a
dropped announcement costs a beat and never a stranded subject.

#### Scenario: The system reaches the same states without any announcement

- **WHEN** every publication of a topic is suppressed
- **THEN** every subject still reaches the same states, more slowly, because each
  transition is reachable from the world the state's conditions read

### Requirement: Every wait the hub performs is a state

A subject SHALL stand in a state wherever the hub asks for something and time passes
before the answer, and that state's action SHALL perform the work under a context the
machine cancels when the subject leaves. A wait carried out inside a verb, a sweep, or
an observer's bookkeeping SHALL be expressed as a state instead.

A transition SHALL take no time and SHALL carry no work.

#### Scenario: A pod being launched is a state

- **WHEN** the hub starts an agent's pod
- **THEN** the agent stands in a launching state until the pod is observed, and a
  launch that never completes is bounded by that state rather than by a sweep

#### Scenario: A merge in flight is a state

- **WHEN** a human's merge intent is recorded for an approved pull request
- **THEN** the pull request stands in `pr/merging` while the merge runs, its result
  moves it on, and a hub that dies mid-merge leaves it recoverable through the
  state's own orphan exit

#### Scenario: Work in flight survives a restart

- **WHEN** the hub restarts while a subject is in a state whose action was running
- **THEN** the machine observes the state was entered before it started with nothing
  running, and moves the subject by that state's declared orphan exit

### Requirement: A verb declares what it may run, never where it lands

A state SHALL declare the verbs available in it, and running one SHALL move nobody.
Where a subject lands after a verb SHALL be derived by following that state's
conditions against the world, so the map holds one account of every transition.

#### Scenario: A verb's effect is decided by the world it changed

- **WHEN** an agent runs a verb its state offers
- **THEN** the verb records its fact, and the agent's next state is whatever its
  conditions conclude from the world that fact changed

#### Scenario: An agent cannot declare where it stands

- **WHEN** an agent attempts to set its own flow state
- **THEN** no verb offers it, because where a subject stands is the machine's alone

### Requirement: A repair is an observed condition

A world in a shape that should not exist SHALL be corrected by a condition on the
subject that holds the mismatch, watched wherever that subject stands. No sweep
SHALL exist whose purpose is to repair state, and no repair SHALL be reachable only
at startup.

#### Scenario: A repair happens when the shape appears

- **WHEN** a hierarchy is split under the agent holding its container
- **THEN** that agent's own map observes the split and yields, at the moment it
  becomes true rather than at the hub's next start

### Requirement: A test reaches a state through one door

A test fixture SHALL reach a state through a single shared function, which writes what
the agent holds and where it stands through the same two writers the hub uses, and
SHALL refuse any state no role's flow declares. A fixture whose SUBJECT is the flow —
what a verb, a verdict or a merge leaves an agent standing in — SHALL run the machine
rather than writing the answer it is asserting.

This closes the gap that hid the stranded reviewer: a fixture writing directly can
construct a world the machine would never produce — a reviewer holding an assigned
review with its pod down — and then assert that it behaves correctly. One door with a
declared-states check is narrower than the hundred call sites it replaced, and wider
than running the machine for every fixture: a fixture may still stand an agent somewhere
the machine would not have put it THIS time, and only the states it could ever produce.

#### Scenario: A fixture cannot build an undeclared state

- **WHEN** a fixture places an agent in a state its role's flow does not declare
- **THEN** the test fails, naming the role and the state, rather than the agent
  silently reading as standing at its role's start

#### Scenario: A fixture about the flow runs the machine

- **WHEN** a test asserts where a verdict, a merge or a gate leaves an agent
- **THEN** it records the fact and looks, so the answer comes from the map rather
  than from the fixture
