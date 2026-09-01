# hub — delta

## ADDED Requirements

### Requirement: One situation describes an agent

The hub SHALL carry an agent's full state in ONE struct — the situation it is in — gathered in one
place. It covers the roster row (role, retired, clear-armed, stopped), the workflow state (phase,
task, container, escalation), the observer's last reading (up, runtime, clients, how long the screen
has stood still, context fill and window), the transient launch intent, and what the agent could be
handed right now.

The project's own state — what work is claimable — SHALL be a struct of its own that an agent's
situation CONTAINS, not a second thing a caller carries beside it. What is claimable describes the
project, not the agent; but what an agent may do depends on it, so it belongs inside the one thing a
caller passes and the surface reads.

Containment is what makes the sharing structural rather than a rule to remember: a project's state is
gathered ONCE and the situations of every agent in it refer to that one reading. The claimable pool is
a single query for a whole roster today, and a situation that re-read it per agent would turn one
query into one per agent on every sweep.

Gathering SHALL otherwise be the struct's own work, so a caller assembles nothing.

No call SHALL reach the container runtime. Everything costly is already held in memory — the
observer's readings and the launch intent — and the rest is local store reads the board already makes
(-> the observer's monopoly).

Nothing else SHALL read those facts to decide something. A caller wanting to know what may happen
asks the surface below; a caller wanting to render reads the board.

#### Scenario: The situation is assembled once

- **WHEN** the hub needs to know what may happen to an agent
- **THEN** one call yields its full situation, taking no arguments beyond the agent's identity

#### Scenario: A roster is judged on one read of the backlog

- **WHEN** the situations of every agent in a project are gathered together
- **THEN** the claimable pool is read once for all of them, not once per agent

#### Scenario: A situation costs no runtime call

- **WHEN** a situation is gathered
- **THEN** no container operation is performed, whatever the agent's state

### Requirement: One surface says what is allowed

The hub SHALL derive the allowed actions from a situation in one place: whether the agent may be
assigned work, nudged toward it, compacted, cleared, reclaimed, or woken; whether it is stalled; and
whether only the user can move it on.

Each action SHALL carry the reason it is unavailable, empty meaning allowed. Reasons rather than
flags because they are what an agent is told and what a user reads, and because a refusal composed at
the call site is a refusal that drifts from the rule that caused it — as `Offered.Blocked` already
establishes for the verbs an agent may type.

The rules SHALL be stated once. Where a rule exists today in more than one place, the surface becomes
its only home and every site asks it. A caller MUST NOT re-derive an answer the surface gives, and a
new rule about what may happen to an agent belongs here rather than at the site that first needs it.

#### Scenario: One rule, one answer, every caller

- **WHEN** two different code paths ask whether an agent may be handed work
- **THEN** both receive the same answer and the same reason, because both asked the same surface

#### Scenario: A refusal explains itself

- **WHEN** an action is unavailable
- **THEN** the surface states why in the words the agent or user is given, rather than a bare flag
  the caller must interpret

### Requirement: A front-end reads a decision, never repeats it

A capability a front-end needs SHALL travel to it as a field the hub has already decided, alongside
the status and the flags the board already carries.

This is not a preference: a front-end links no hub package, so a rule evaluated in the hub cannot be
evaluated again in a front-end without a second copy of it — and a second copy is the drift this
change exists to end.

#### Scenario: The board carries the answer

- **WHEN** a front-end renders something that depends on what an agent may do
- **THEN** it reads a decided field rather than applying a rule of its own

### Requirement: The surface is the only home for these rules

A build check SHALL fail when a rule the surface owns is derived anywhere else, in the manner already
used for who may inject into an agent's session and who may poll the container runtime: the call
sites are enumerated, and one that is not declared fails the build with the reason it is allowed.

Prose has already proved insufficient here. The rule that a board read never probes was stated in the
observer's own header for months and was broken anyway; the guard on nudging a retired agent was added
to one function and omitted from another in the same commit.

#### Scenario: A sixth decider is refused

- **WHEN** new code derives one of these rules from raw facts instead of asking the surface
- **THEN** the build fails, naming the rule and where it belongs
