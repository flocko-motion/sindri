# View: workers — delta

## MODIFIED Requirements

### Requirement: Orphaned runtime is flagged

The workers view SHALL flag orphaned runtime — a pod or worktree with no roster
entry — as a warning distinct from any agent row, and SHALL offer to remove it,
confirming first. Orphans SHALL NOT be rendered as if they were declared agents.

#### Scenario: Orphan warning

- **WHEN** a pod exists with no matching roster entry
- **THEN** the view shows an "orphaned agent" warning, not a normal agent row

#### Scenario: Removal is offered, not merely described

- **WHEN** the user acts on a flagged orphan
- **THEN** the view asks for confirmation and then removes it, rather than printing a
  command for the user to run themselves
