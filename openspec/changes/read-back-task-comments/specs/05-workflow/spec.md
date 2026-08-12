# workflow — delta

## ADDED Requirements

### Requirement: A task's comment thread is readable where the task is shown

An interface that shows a task's own detail SHALL render that task's comment thread. This covers
the agent-facing task view, the host CLI's task info, and the TUI's task detail pane. Each comment
SHALL carry its author, its body, its creation time, and its source, and the thread SHALL read
oldest-first. An agent SHALL be able to read back a comment written on the task it holds.

A thread with no comments SHALL render nothing: no heading and no count.

This covers a task shown as the subject of its own detail. A task rendered as a cross-reference
from elsewhere, such as a peek at another task or a PR's linked task, is served from the board
snapshot, which carries no thread, and is not required to show one.

#### Scenario: An agent reads its own task

- **WHEN** an agent asks for the task it holds
- **THEN** the view shows the task's comment thread, oldest-first, each comment with its author,
  body, time and source

#### Scenario: An agent reads back what it wrote

- **WHEN** an agent comments on its task and then asks for that task again
- **THEN** its own comment is in the thread it is shown

#### Scenario: The source is named

- **WHEN** a comment came from the task's upstream GitHub issue
- **THEN** the rendered comment names `github` as its source, distinguishing it from one held
  only in the hub

#### Scenario: Nothing has been said

- **WHEN** a task has no comments
- **THEN** its detail shows no comment heading and no count
