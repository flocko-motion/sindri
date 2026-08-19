# 05-workflow — delta

## MODIFIED Requirements

### Requirement: The reviewer pool spans projects

Assignment SHALL consider `_global` reviewers alongside the project's own when looking for a reviewer
to hand an unclaimed review to. A pool read from one project's roster leaves an idle reviewer in
another repo unreachable — which is waste no reclaiming of idle pods can recover, since the
constraint is identity rather than memory.

Where both a local and a global reviewer are free, either MAY take the review. A repo that keeps its
own reviewer expects it to be used, so a local one SHALL be preferred; the global pool is what
answers when there is none.

A global reviewer holding a review SHALL be found by the same query wherever that review lives. What
a reviewer is reviewing is a fleet-wide question once the reviewer is fleet-wide, so a project-scoped
read of its held review reports nothing for an agent that is plainly busy.

The workspace a review is checked out into SHALL be resolved from the reviewer's own record rather
than from the project the PR belongs to. A global reviewer is on no project roster, and the existing
lookup already fails loudly in that case — it would fail on every review.

#### Scenario: A global reviewer takes another project's review

- **WHEN** a project has an unclaimed review and no free reviewer of its own, and the pool has one
- **THEN** the global reviewer is assigned it, and its workspace is resolved from its own record

#### Scenario: A local reviewer is preferred

- **WHEN** both a local and a global reviewer are free for the same review
- **THEN** the local one takes it

#### Scenario: A busy global reviewer is seen to be busy

- **WHEN** a global reviewer holds a review in one project and any project's board is read
- **THEN** it reads as reviewing that PR rather than as idle

## ADDED Requirements

### Requirement: Reviews queue when the pool is busy

A review that finds no free reviewer SHALL wait, and SHALL be handed out when one becomes free. A
bounded pool means concurrent reviews are bounded, and queuing is the correct answer — the fleet
already treats its single run slot this way.

Waiting SHALL be visible: a review nobody has picked up is a fact the board already carries, and it
must not become invisible for being queued behind a busy pool rather than an absent reviewer.

#### Scenario: Every reviewer is busy

- **WHEN** a review is requested and no reviewer, local or global, is free
- **THEN** it waits, stays visible as waiting, and is handed out when one frees
