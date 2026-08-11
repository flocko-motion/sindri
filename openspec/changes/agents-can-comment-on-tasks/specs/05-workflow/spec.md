# Workflow — delta

## RENAMED Requirements

- FROM: `### Requirement: Communication via comments`
- TO: `### Requirement: Communication via session injection`

## MODIFIED Requirements

### Requirement: Communication via session injection

Messages to an agent SHALL be delivered by the hub injecting them into the
agent's session, each stamped with its source. A human reaches an agent via the
hub-mediated `tell` channel; another agent reaches it only by acting on a shared
object whose consequence the hub routes. The agent SHALL see these as tagged
lines in its single input stream.

#### Scenario: Human nudge

- **WHEN** a human sends a message to an agent
- **THEN** the hub injects it into that agent's session tagged `[user]`

#### Scenario: Reviewer feedback

- **WHEN** the reviewer rejects an agent's PR with feedback
- **THEN** the hub routes the feedback to the owning agent's session tagged
  `[reviewer]`

## ADDED Requirements

### Requirement: An agent can comment on a task

An agent SHALL be able to record a durable comment on a task, through the same
comment thread a human's `task comment` writes to and reads from. This is
distinct from session injection (above) and from the activity log: a comment is
discussion attached to the task itself, read by whoever next opens it, not a
message delivered into a session and not a private audit trail. Each comment
SHALL be attributed to the agent that wrote it.

Scope SHALL match what the agent's role already sees: a worker MAY comment only
on the task it holds or the container it is working inside; a reviewer MAY
comment only on the task of the PR it is reviewing; a planner or a coauthor MAY
comment on any task in its project, since both already read the whole backlog.
An agent SHALL NOT be offered the capability when it has nothing to comment on
(a worker holding no task, a reviewer reviewing nothing).

#### Scenario: A worker records a finding on its own task

- **WHEN** a worker comments on the task it currently holds
- **THEN** the comment is recorded on that task's thread, attributed to the
  worker, and visible to a human reading the task's detail

#### Scenario: A worker cannot comment on a task it does not hold

- **WHEN** a worker attempts to comment on a task other than the one it holds
  or the container it is working inside
- **THEN** the attempt is refused

#### Scenario: A reviewer comments on the task it is reviewing

- **WHEN** a reviewer comments while reviewing a PR
- **THEN** the comment is recorded against the task that PR belongs to

#### Scenario: A gh- task's comment reaches the issue

- **WHEN** an agent comments on a task backed by a GitHub issue
- **THEN** the comment is posted to that issue, the same as a human's would be

#### Scenario: The verb is absent with nothing to comment on

- **WHEN** a worker holds no task, or a reviewer is reviewing nothing
- **THEN** the comment capability does not appear in that agent's available
  commands
