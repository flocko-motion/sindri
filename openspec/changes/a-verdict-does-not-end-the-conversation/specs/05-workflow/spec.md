# 05-workflow — delta

## ADDED Requirements

### Requirement: A reviewer's reach outlives its verdict

A reviewer SHALL be able to comment on the task of any PR it has recorded a verdict on, not only the
review it currently holds. Recording the verdict is what ENDS the held review, so a scope defined by
the held review shuts the verb at the exact moment the reviewer acquires afterthoughts — a second look
at a design choice, a consequence it missed, or an answer to the user asking about the rejection.

The finding SHALL have somewhere durable to go, and that place is the task. A comment on the task is
read by whoever next opens it — a later worker, a second reviewer, a planner, or the same reviewer
after a compaction. A message is addressed to one agent, is seen by nobody else, and nothing leads a
future reader to it; pushing a reviewer to mail for a finding produces no record anyone will find.

The scope SHALL stay bounded by what the reviewer has read. Having ruled on a PR is that proof, so a
task it has never reviewed SHALL still be refused. One resolution of that scope SHALL serve the gate
that offers the verb, an id-less comment, and the check on an explicit id: a verb offered by one rule
and refused by another is worse than one that is simply absent.

An id-less comment SHALL mean the newest task in reach — the held review while there is one, otherwise
the latest verdict — and the others SHALL remain reachable by name.

Where a reviewer holds no review and has ruled on nothing, the refusal SHALL name what would open the
verb rather than only stating that it is shut.

The receipt for a verdict SHALL say that the task remains open to comment. It is what the reviewer
reads at the moment an afterthought is most likely, and a reach nobody is told about is one nobody
uses.

#### Scenario: An afterthought after a rejection

- **WHEN** a reviewer has rejected a PR and then comments
- **THEN** the comment is recorded on that PR's task, attributed to the reviewer, where any later
  reader of the task finds it

#### Scenario: The held review comes first

- **WHEN** a reviewer holding a new review comments without naming a task
- **THEN** the comment lands on the held review's task, and the task of its earlier verdict is still
  reachable by naming it

#### Scenario: Still no wandering the backlog

- **WHEN** a reviewer names a task it has never reviewed
- **THEN** the comment is refused, and the refusal names the tasks it can comment on

#### Scenario: Nothing reviewed yet

- **WHEN** a reviewer holds no review and has recorded no verdict
- **THEN** the verb is unavailable, and the reason names approving or rejecting a PR as what opens it

#### Scenario: The verdict receipt says so

- **WHEN** a reviewer's verdict is recorded
- **THEN** what it is told names the comment verb and the task as where a finding belongs
