# View: Workers — delta

## MODIFIED Requirements

### Requirement: Reviewer distinct

Each agent SHALL appear with its own role — worker, reviewer, or planner — taken
from the hub's state. The review agent and the planner SHALL each be shown with
their own role and SHALL NOT be rendered as dwarf workers. Because a planner is
never assigned a backlog task, its status SHALL be one of down, idle, or submitted
(it is never "working").

#### Scenario: Listing with a reviewer

- **WHEN** the reviewer and dwarf workers are listed together
- **THEN** the reviewer is shown with the reviewer role, not as a dwarf worker

#### Scenario: Listing with a planner

- **WHEN** a planner is listed alongside workers and a reviewer
- **THEN** it is shown with the planner role, and its status is idle/submitted/down
  rather than working
