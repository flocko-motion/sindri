# Workers View — delta

## MODIFIED Requirements

### Requirement: Worker rows

The workers view SHALL render from JSON the hub delivers — name, role, status,
current task, merge-intent, workspace, and branch — not from data the UI gathers
itself. The CLI and TUI SHALL show the same fields from the same hub payload.

#### Scenario: Running worker

- **WHEN** a worker is running on a task
- **THEN** its row shows the task and branch, taken from the hub's state payload

#### Scenario: CLI and TUI parity

- **WHEN** the CLI and TUI both list workers
- **THEN** they present the same fields, both sourced from the hub

### Requirement: Stopped workers visible

The workers view SHALL show status as reported by the hub, which owns live state.
It SHALL distinguish at least: running, idle (waiting for the hub), and
no-workspace (rebuildable from the roster). Status SHALL come from the hub, not
from inferring container or worktree position.

#### Scenario: An idle worker

- **WHEN** an agent has registered its work and is waiting
- **THEN** its row shows idle, distinct from a running agent

#### Scenario: A rebuildable agent

- **WHEN** a rostered agent has no workspace
- **THEN** its row shows "no workspace" rather than being hidden

## ADDED Requirements

### Requirement: Orphaned runtime is flagged

The workers view SHALL flag orphaned runtime — a pod or worktree with no roster
entry — as a warning distinct from any agent row, and SHALL show the proposed
shell command to remove it. Orphans SHALL NOT be rendered as if they were declared
agents.

#### Scenario: Orphan warning

- **WHEN** a pod exists with no matching roster entry
- **THEN** the view shows an "orphaned agent" warning with a suggested removal
  command, not a normal agent row

### Requirement: Per-worker activity timeline

The workers view SHALL be able to show a per-worker activity timeline rendered
from the hub's durable activity log — the worker's actions, the messages it sent
and received over the socket, its merge-intents and verdicts, and its status
transitions — so a human can understand what the worker has been doing. The
freeform pane chat is not part of this timeline.

#### Scenario: Inspecting a worker's history

- **WHEN** a human opens a worker's activity timeline
- **THEN** it shows that worker's logged actions, socket messages, merge-intents,
  and status transitions in order

#### Scenario: Sourced from the hub log

- **WHEN** the timeline is shown
- **THEN** its entries come from the hub's append-only activity log, not from the
  agent's freeform terminal output
