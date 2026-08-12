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
SHALL be attributed to the agent that wrote it: in its own field for a task
sindri owns the thread of, and — where the thread instead belongs to an
external tracker with no such field, posting under whichever identity the
tracker adapter authenticates with — named in the comment's own text instead,
so the attribution is never silently lost or misread as the human's.

Scope SHALL match what the agent's role already sees: a worker MAY comment only
on the task it holds or the container it is working inside; a reviewer MAY
comment only on the task of the PR it is reviewing; a planner or a coauthor MAY
comment on any task in its project, since both already read the whole backlog.
An agent SHALL NOT be offered the capability when it has nothing to comment on
(a worker holding no task, a reviewer reviewing nothing).

Naming the task SHALL be OPTIONAL wherever the agent's own state already names
one: a worker's comment defaults to the task it is working, a reviewer's to the
task of the PR under review. Only a planner or a coauthor, who have no single
current task, SHALL be required to name one. The advertised argument form SHALL
match: an argument a role never supplies does not appear in the help that role
reads. Where an agent has two tasks in reach — a worker inside a feature holds
both the container and the subtask it is on — the default SHALL be the subtask,
the container SHALL stay reachable by its id, and the reply SHALL name the task
the comment landed on.

#### Scenario: A worker records a finding on its own task

- **WHEN** a worker comments on the task it currently holds
- **THEN** the comment is recorded on that task's thread, attributed to the
  worker, and visible to a human reading the task's detail

#### Scenario: A worker comments without naming its task

- **WHEN** a worker comments giving only the text
- **THEN** the comment is recorded on the task it is working, and the reply
  names that task

#### Scenario: A worker inside a feature reaches either task

- **WHEN** a worker holding a feature comments giving only the text, and then
  again naming the feature's id
- **THEN** the first lands on the subtask it is on, the second on the feature,
  and each reply says which of the two it was

#### Scenario: A planner is still asked for an id

- **WHEN** a planner reads the help for the comment capability
- **THEN** the form it is shown requires a task id, since no single task is
  implied by its state

#### Scenario: A worker cannot comment on a task it does not hold

- **WHEN** a worker attempts to comment on a task other than the one it holds
  or the container it is working inside
- **THEN** the attempt is refused

#### Scenario: A reviewer comments on the task it is reviewing

- **WHEN** a reviewer comments while reviewing a PR
- **THEN** the comment is recorded against the task that PR belongs to

#### Scenario: A gh- task's comment reaches the issue, naming its real author

- **WHEN** an agent comments on a task backed by a GitHub issue
- **THEN** the comment is posted to that issue under whichever account the `gh`
  adapter authenticates with, its text naming the agent that actually wrote
  it — never silently read as the human's

#### Scenario: A human's comment on a gh- task is posted unchanged

- **WHEN** a human comments on a task backed by a GitHub issue
- **THEN** the comment is posted verbatim, with no agent identity to name

#### Scenario: The verb is absent with nothing to comment on

- **WHEN** a worker holds no task, or a reviewer is reviewing nothing
- **THEN** the comment capability does not appear in that agent's available
  commands
