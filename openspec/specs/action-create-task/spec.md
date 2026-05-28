# Action: Create Task

## Purpose

Defines creating a task, optionally linked to a spec, independent of any
interface (`sindri task new`, the TUI new-task modal, and the agent's flow all
perform this one action).

## Requirements

### Requirement: Create a task

Creating a task SHALL set its title, type, and priority, and MAY set a
description. Creation SHALL go through the td adapter, never a direct CLI call.

#### Scenario: Minimal create

- **WHEN** a task is created with a title
- **THEN** a task is recorded via the td adapter with the given title

### Requirement: Optional spec link

Creation SHALL support an optional spec link: attaching a `spec:<name>` label at
creation makes the task the implementing task for that spec.

#### Scenario: Linked create

- **WHEN** a task is created for spec `add-auth`
- **THEN** it carries the `spec:add-auth` label and pairs with that spec on the board

### Requirement: Optional review gate

Creation SHALL support an optional review gate (e.g. `require-review-code`) so
the task must pass review before merge.

#### Scenario: Gated create

- **WHEN** a task is created with code review required
- **THEN** it carries `require-review-code`
