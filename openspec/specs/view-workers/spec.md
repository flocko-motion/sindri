# View: Workers

## Purpose

Defines the workers view — the list of sindri workers and what each is doing —
independent of any interface. `sindri worker list` and the TUI workers view are
renderings of this one definition.
## Requirements
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

### Requirement: Reviewer distinct

The review agent SHALL appear distinctly from the dwarf workers (a separate
role), not as one of them.

#### Scenario: Listing with a reviewer

- **WHEN** the reviewer and dwarf workers are listed together
- **THEN** the reviewer is shown with the reviewer role, not as a dwarf worker

### Requirement: Loading state distinct from empty

Before the first board refresh has applied, the workers view SHALL show a
"Loading workers…" placeholder, distinct from the empty-state placeholder used
when a refresh has applied but no workers exist. The loading placeholder SHALL
replace any empty-state text in this window so the user is never misled into
thinking there are no workers before the data has arrived.

#### Scenario: First frame after startup

- **WHEN** the TUI is rendered after the window has sized itself but before
  the first refresh has applied
- **THEN** the workers panel shows "Loading workers…" instead of "No workers"

#### Scenario: Refresh applied, no workers

- **WHEN** a refresh has applied and the project has no workers
- **THEN** the workers panel shows its empty-state placeholder ("No workers")

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

