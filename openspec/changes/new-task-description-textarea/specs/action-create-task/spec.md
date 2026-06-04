# Action: Create Task — delta

## MODIFIED Requirements

### Requirement: Create a task

Creating a task SHALL set its title, type, and priority, and MAY set a
description. The description input SHALL be multi-line: long
descriptions SHALL wrap onto multiple rows rather than scrolling
horizontally in a single-row input. Creation SHALL go through the td
adapter, never a direct CLI call.

#### Scenario: Minimal create

- **WHEN** a task is created with a title
- **THEN** a task is recorded via the td adapter with the given title

#### Scenario: Multi-line description

- **GIVEN** the user types a description that exceeds the input width
- **WHEN** the modal renders
- **THEN** the description wraps onto additional rows
- **AND** the modal does NOT scroll the description horizontally

## ADDED Requirements

### Requirement: Submit shortcut and dual enter semantics

The new-task modal SHALL accept `ctrl+s` as a global submit
shortcut, available from every field. The plain `enter` key SHALL
submit the form when the cursor is on the title, type, priority, or
review field (its existing behavior), and SHALL insert a newline when
the cursor is in the multi-line description field — pressing enter to
start a new paragraph SHALL NOT create the task half-typed.

#### Scenario: Submit via ctrl+s from any field

- **GIVEN** the cursor is on any field
- **WHEN** the user presses `ctrl+s`
- **THEN** the form is submitted (subject to validation)

#### Scenario: Enter on the title submits

- **GIVEN** the cursor is on the title field
- **AND** the title meets validation
- **WHEN** the user presses `enter`
- **THEN** the form is submitted

#### Scenario: Enter in description inserts a newline

- **GIVEN** the cursor is in the description field
- **WHEN** the user presses `enter`
- **THEN** a newline is inserted in the description
- **AND** the form is NOT submitted
