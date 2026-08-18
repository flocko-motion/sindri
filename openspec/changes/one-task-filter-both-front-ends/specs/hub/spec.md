# hub — delta

## ADDED Requirements

### Requirement: Task filtering is a function of the exchange format

Which tasks a view shows SHALL be decided by one predicate in the exchange package, so
the hub and every front-end admit the same tasks under the same word and no interface
derives its own. The filters SHALL be exactly four:

- **open** — not done (open, in_progress, in_review)
- **closed** — the done segment (closed, approved, merged)
- **all** — every task, whatever its status
- **active** — the union of open with every task whose status changed inside the
  active window

The active window SHALL be a single constant in the exchange package, so no interface
can hold a different idea of how recent "recently" is. A task carrying no timestamp
SHALL NOT count as recently changed: absence of a date is not evidence of recency.
Every task source SHALL date the tasks it mirrors, so that rule excludes only what is
genuinely unknown rather than a whole source.

Every front-end SHALL offer all four filters — the CLI as a flag on its task listing,
the TUI as its cycle — since a filter one interface has and the other cannot reach is
behaviour missing from that interface. Which filter a view opens on is its own choice:
a listing is a record of what exists, a live screen a view of what is happening.

#### Scenario: Both front-ends filter alike

- **WHEN** the CLI lists tasks under a filter and the TUI shows the same tasks under
  the same filter
- **THEN** both call the same predicate from the exchange package and admit the same
  tasks

#### Scenario: Active keeps work that has just finished

- **WHEN** a task is closed and then the view is filtered to active
- **THEN** it is still shown while it is inside the active window, and drops out once
  it is past it

#### Scenario: An undated task

- **WHEN** a done task carries no timestamp
- **THEN** it is absent from active, and present under closed and all

#### Scenario: A filter is named that does not exist

- **WHEN** a user asks for a filter outside the four
- **THEN** they are told which filters exist, rather than being shown a listing that
  reads as an empty backlog
