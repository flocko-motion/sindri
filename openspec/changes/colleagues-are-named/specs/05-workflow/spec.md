# 05-workflow — delta

## ADDED Requirements

### Requirement: A planner sees the staff of its own repo

A planner SHALL be able to list the agents in its own project — name, role, and what each is
holding — as context for the work it plans. The listing SHALL be scoped to the planner's own
project and SHALL NOT enumerate agents in any other project.

This is visibility, not addressing: mail reaches any agent by name regardless of project, but a
name from another repo reaches a planner only through the user's own instruction, never through a
lookup this listing performs. An agent may talk to a colleague it has been told about, and SHALL NOT
be given a way to browse the fleet for one.

What a colleague holds SHALL be stated in terms that decide whether to wait for it: a reviewer's
open review, a worker's task or the feature it is working through, and whether the agent is retired
and so finishing what it holds rather than taking anything new.

#### Scenario: A planner asks who else is working

- **WHEN** a planner lists its repo's staff
- **THEN** it sees every agent in its own project, each with its role and what it currently holds

#### Scenario: The listing stops at the project boundary

- **WHEN** other projects have agents of their own
- **THEN** none of them appear in the listing

#### Scenario: A retired colleague reads differently from a free one

- **WHEN** the listing includes a retired agent
- **THEN** it says so, distinct from an agent that is simply idle

### Requirement: A reviewer is told whose work it is reviewing

The directive handing a PR to a reviewer SHALL name the agent that wrote it, in the sentence that
says what to do — the moment the name is useful. The author is otherwise reachable only by going
looking, and nothing suggests looking, so verdicts are written about nobody.

Where the record carries no author, the directive SHALL read as it did before rather than claiming
one.

#### Scenario: A PR is handed to a reviewer

- **WHEN** a reviewer is given a PR to review
- **THEN** the directive names its author alongside the task it is for

#### Scenario: The same review, asked for again

- **WHEN** a reviewer re-asks for its held review
- **THEN** that directive names the author too, not only the one that first handed it over

### Requirement: A task view names the agent holding it

A task view SHALL name the agent behind the task — a listing as a column, a single task in full as
a field — so that a reader planning around the work can see who is doing it. The name SHALL come
from one rule wherever it is rendered, the dashboard's detail pane, the host CLI and the
agent-facing task verb alike, since it is one fact shown in several places.

That rule SHALL name the agent holding the task, the agent holding the feature it belongs to, and
failing either, the author of its open pull request. A submitted task is exactly when the name is
most wanted — the work is done and the question is who to ask about it — and it is exactly when the
holder has moved on and only the PR remembers.

Among agents this SHALL be shown to the roles that work around other people, a planner and a
coauthor. A worker sees only the task it holds, and a reviewer is told the author by the directive
that hands it the pull request, so neither is given a column that would make a task view into a
directory of the fleet.

#### Scenario: A task being worked

- **WHEN** an agent holds a task
- **THEN** the listing and the task's own view both name it

#### Scenario: A task under review

- **WHEN** a task's work has been submitted and no agent holds the task any longer
- **THEN** the views name the agent that submitted it

#### Scenario: A task nobody is working

- **WHEN** no agent holds a task and it has no open PR
- **THEN** the field is shown as empty rather than borrowing a name from elsewhere
