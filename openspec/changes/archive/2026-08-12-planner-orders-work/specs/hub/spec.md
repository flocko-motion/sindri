# hub — delta

## ADDED Requirements

### Requirement: A planner may order work, never authorise it

A planner SHALL be able to express the sequence of the work it plans: to propose a priority on a
task it creates, and to change the priority of a task afterwards.

No planner action SHALL change whether a task is claimable. A task is claimable when it carries a
priority AND the approval gate is not holding it, so the rule is one boundary rather than a list of
cases: a planner MAY move a priority between values, and MAY rate a task the gate still holds; it
SHALL NOT cross between unrated and rated on a task the gate would let out, because setting a first
priority there releases work and clearing one withdraws work already released.

A refusal SHALL give that reason, so a planner can tell a rule from a fault.

Where a proposal is created carrying a priority, it SHALL NOT be claimable at any point between its
creation and its gating — the ordering of those writes is part of the guarantee, not an artefact of
it.

A rating a planner makes SHALL NOT be announced to workers as available work, since by the rule
above it never makes any.

An interface offering the human the approval of a rated task SHALL show the priority it carries, so
the sequence being authorised is visible at the moment it takes effect. An interface offering an
action the caller may not perform SHALL NOT offer it: the flow that invites a human to approve a
task they have just rated is a human flow, and a planner SHALL NOT be led into it.

#### Scenario: Rating a proposal

- **WHEN** a planner sets a priority on a task still awaiting approval
- **THEN** the priority is recorded, the task remains unclaimable, and the planner is told so

#### Scenario: Re-ordering approved work

- **WHEN** a planner changes the priority of an approved task that already carries one
- **THEN** the new priority is recorded and the task remains claimable, as it was

#### Scenario: Rating an approved task that has none

- **WHEN** a planner sets a priority on an approved task carrying none
- **THEN** it is refused, nothing is written, and the reason given is that it would release the task

#### Scenario: Clearing the priority of approved work

- **WHEN** a planner would clear the priority of an approved, rated task
- **THEN** it is refused, and the reason given is that it would withdraw released work

#### Scenario: A proposal created with a priority

- **WHEN** a planner creates a task carrying a priority
- **THEN** the task awaits approval and no worker can claim it

#### Scenario: Approving a task a planner rated

- **WHEN** the human approves a task that carries a priority
- **THEN** that priority is named as part of the approval, and no further rating is asked for
