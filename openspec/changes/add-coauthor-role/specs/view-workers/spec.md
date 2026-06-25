# View: Workers — delta

## MODIFIED Requirements

### Requirement: Reviewer distinct

Each agent SHALL appear with its own role — worker, reviewer, planner, or
coauthor — taken from the hub's state, and SHALL NOT be collapsed into the dwarf
workers. The review agent and the coauthor SHALL each be shown with their own
role. Because a coauthor is never assigned a backlog task and never opens a
managed PR, its status SHALL be one of down, idle, or collab — never "working" or
"submitted".

#### Scenario: Listing with a reviewer

- **WHEN** the reviewer and dwarf workers are listed together
- **THEN** the reviewer is shown with the reviewer role, not as a dwarf worker

#### Scenario: Listing with a coauthor

- **WHEN** a coauthor is listed alongside workers and a reviewer
- **THEN** it is shown with the coauthor role, and its status is down/idle/collab
  rather than working or submitted
