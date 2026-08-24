# 05-workflow — delta

## ADDED Requirements

### Requirement: A reviewer that stops reading is noticed

The stall rule SHALL count a reviewer holding a review. `reviewing` MUST NOT be treated as a waiting
phase: `submitted` and `gating` are exempt because they exist to wait for somebody else, while a
reviewer that holds a PR is supposed to be reading it. A reviewer whose screen stands still past the
stall dwell is therefore stalled, on the same evidence as any other agent.

The nudge SHALL name the PR the reviewer holds. A reviewer's hold lives in its review row and in no
state field — assigning a review writes the phase alone — so a nudge that names only a task or a
feature has nothing to say and sends nothing. Both halves of this SHALL land together, since counting
a reviewer as stalled while the nudge still bails changes nothing observable.

Where a reviewer holds no live review, nothing SHALL be sent. The PR is read from the review row
rather than assumed from the phase, so a phase left behind by a released review produces silence
instead of an invented id.

The board SHALL show a stalled reviewer as stalled, from the same observation the nudge reads. That
is what makes the condition visible: under the phase alone it read as an agent quietly reviewing,
hours after it stopped.

#### Scenario: A reviewer's screen stands still

- **WHEN** a reviewer holds an assigned review and its screen has not changed for the stall dwell
- **THEN** it is stalled, it is prodded once for that spell, and the prod names the PR it holds

#### Scenario: The board agrees with the prod

- **WHEN** that same reviewer's row is rendered
- **THEN** it reads `stalled` rather than `reviewing`

#### Scenario: Nothing held, nothing said

- **WHEN** an agent's phase says reviewing but no live review row is assigned to it
- **THEN** no nudge is sent

#### Scenario: A reviewer correctly waiting

- **WHEN** a reviewer's session is waiting on the user, or it is signed out
- **THEN** it is not stalled, exactly as for any other role
