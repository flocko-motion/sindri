# Workers View — delta

## MODIFIED Requirements

### Requirement: Stopped workers visible

The workers view SHALL show reconciled agent status derived from the index, not
just live container state. It SHALL visibly distinguish at least: running,
crashed mid-task, idle, no-workspace (rebuildable), and orphan (a container or
worktree with no index entry).

#### Scenario: A crashed worker

- **WHEN** an indexed agent has a task but no running container
- **THEN** its row shows a "crashed" status distinct from a cleanly stopped one

#### Scenario: A rebuildable agent

- **WHEN** an indexed agent has no workspace directory
- **THEN** its row shows a "no workspace" status rather than being hidden
