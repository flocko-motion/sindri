# Architecture

## Purpose

Defines the structural rules every part of sindri must follow. The core
principle: application logic is independent of any user interface, the same
logic backs all interfaces, and external tools are reached only through
adapters. This keeps logic testable headless and keeps interfaces
interchangeable.

## Requirements

### Requirement: Headless logic

Application logic SHALL be implemented independently of any user interface and
be fully usable without one — including driven directly from unit tests.

#### Scenario: Logic exercised from a test

- **WHEN** a behavior is verified
- **THEN** it is invoked through the logic layer with no CLI, TUI, or GUI present

#### Scenario: Logic has no UI imports

- **WHEN** a logic package is compiled
- **THEN** it imports no UI package (CLI/TUI/GUI) and no rendering package

### Requirement: Thin UI layer

A user interface (CLI, TUI, GUI) SHALL be a thin wrapper over the logic layer.
Every state change SHALL be a call into the logic layer; a UI SHALL NOT
implement domain logic of its own.

#### Scenario: UI changes state

- **WHEN** a user action mutates state
- **THEN** the UI calls a logic-layer function rather than mutating state itself

### Requirement: UI-neutral rendering module

Generic presentation helpers SHALL live in a UI-neutral rendering module shared
by all interfaces, not inside any specific UI. This covers text formatting,
mapping a state to a color, and similar concerns.

#### Scenario: Same state, same styling

- **WHEN** two interfaces display the same item state
- **THEN** both obtain its formatting from the shared rendering module

### Requirement: External adapters are isolated

Integrations with external services or wrapped external tools SHALL be
implemented as dedicated adapter modules connected to the internal logic.
Internal logic SHALL remain free of the implementation details of external
services.

#### Scenario: Internal logic needs external data

- **WHEN** internal logic needs data from an external tool or API
- **THEN** it goes through an adapter module rather than calling the tool directly

### Requirement: Interchangeable interfaces

All UIs SHALL present the same data as similarly as possible. When one interface
shows an item's details, every other interface SHALL show the same fields for
that item.

#### Scenario: Item detail parity

- **WHEN** the CLI and the TUI both display the same item
- **THEN** they present the same set of fields for it

### Requirement: Views and actions are specified

Each view and each action SHALL be defined as an openspec specification, so that
all UI variants align to a single definition.

#### Scenario: A view is added or changed

- **WHEN** a view or action is introduced or modified
- **THEN** an openspec specification defines it that all UIs implement

### Requirement: Layer types and their rules

Every source file's `type:` (see File headers) SHALL be one of the following,
and each type MUST obey its dependency rules:

- `logic` — domain model and rules. MUST NOT import UI, rendering, adapters, or
  assembly packages, and MUST NOT call external tools/APIs. The bottom layer.
- `adapter` — wraps one external tool or API. MAY import `logic` for its types.
  MUST NOT import UI or rendering, and MUST NOT contain domain rules.
- `assembly` — composes adapters + logic into the app's state. MAY import
  `logic` and `adapter`. MUST NOT import UI or rendering.
- `rendering` — maps state to presentation (styles, formatting). MAY import
  `logic` types. MUST NOT import UI, adapters, or assembly, and holds no data logic.
- `ui` — a specific interface (TUI/GUI). MAY import `logic`, `assembly`,
  `rendering`, and `adapter` (for mutations). MUST NOT implement domain logic.
- `command` — a CLI subcommand wrapper. Same dependency freedom as `ui`; thin.
- `entrypoint` — wires a command tree and dispatches. No logic.

#### Scenario: UI contains no logic

- **WHEN** a `ui` or `command` file changes state
- **THEN** it calls `logic`/`assembly`/`adapter`, never reimplementing the rule

#### Scenario: Logic stays pure

- **WHEN** a `logic` file is compiled
- **THEN** it imports no adapter, assembly, rendering, or UI package, and calls
  no external tool

### Requirement: File length limit

No source file SHALL exceed 700 lines of code.

#### Scenario: A file would exceed the limit

- **WHEN** a source file would grow past 700 LOC
- **THEN** it is split into smaller, focused files

### Requirement: Documented directory structure

The source directory structure SHALL be documented in openspec and kept current
as the layout evolves.

#### Scenario: Locating a responsibility

- **WHEN** a developer needs to find where a responsibility lives
- **THEN** the documented structure in this spec names the directory for it

### Requirement: File headers

Every source file SHALL begin with a structured header with four fields:

- `package:` — the package, optionally `package / file` to name the file's role
- `type:` — one of: `logic`, `adapter`, `assembly`, `rendering`, `ui`,
  `command`, `entrypoint`
- `job:` — one or two lines on what this file does
- `limits:` — what it deliberately does NOT do, each pointing at the neighbour
  package responsible (`-> package X`)

#### Scenario: Reading a file's role

- **WHEN** a developer opens any source file
- **THEN** its header states package, type, job, and limits with neighbour pointers

#### Scenario: Header example

- **WHEN** the td adapter is opened
- **THEN** its header reads, in effect:
  `package: td` / `type: adapter (external tool)` /
  `job: wraps the td CLI, converting td JSON to issue.Task` /
  `limits: doesn't assemble issues (-> board) nor render them (-> render)`

## Source layout

The layered structure that realizes the rules above:

- `internal/issue/` — **logic / bottom primitive.** The domain model (`Issue` =
  a task and/or an openspec change, with worker name and PRs) and all state and
  label logic. Pure: imports no other internal package, no UI, no rendering.
- `internal/board/` — **assembly / refresh.** The single data path that gathers
  the fractured sources (td tasks, openspec changes, workers, PRs) and derives
  one coherent `[]issue.Issue` via the pure `issue.Assemble`. Sits above
  `issue` and depends on the adapters it must not.
- `internal/render/` — **UI-neutral rendering.** Maps state to styling (status
  colors, gate marks). Shared by every interface; contains no interface code.
- `internal/ghlocal/`, `internal/openspec/` — **adapters.** Wrap external tools
  (the local PR store, the openspec CLI). Internal logic reaches these tools
  only here.
- `internal/worker/` — **adapter + lifecycle.** Wraps podman/git worktrees for
  agent containers.
- `cmd/sindri/` — **CLI interface.** Thin Cobra wrappers that call `board`/
  `issue` and render via `render`.
- `internal/tui/` — **TUI interface.** Thin Bubble Tea wrappers over the same
  `board`/`issue` state, rendering via `render`.
- `cmd/gh/`, `internal/ghlocal/cmd/` — **the agent-facing `gh` CLI** (sindri-
  local), the workflow engine agents drive inside containers.
