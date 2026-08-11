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

It SHALL write nothing to a terminal. A condition worth reporting SHALL be returned to
its caller or recorded in the log the interfaces read, never printed to standard
output or standard error as a user-facing message, because the core has no terminal to
own.

#### Scenario: Logic exercised from a test

- **WHEN** a behavior is verified
- **THEN** it is invoked through the logic layer with no CLI, TUI, or GUI present

#### Scenario: Logic has no UI imports

- **WHEN** a logic package is compiled
- **THEN** it imports no UI package (CLI/TUI/GUI) and no rendering package

#### Scenario: A core warning is logged, not printed

- **WHEN** the core encounters something a human should know about
- **THEN** it records it where the interfaces can render it, and prints nothing itself

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

Presentation SHALL NOT live in the hub. Display words for a code or status, glyphs
and icons, help text, colour palettes, and hints that exist only for drawing SHALL
belong to the shared rendering module, and the hub SHALL serve the data they are
derived from. The rendering module SHALL NOT import a service package to obtain them.

#### Scenario: Same state, same styling

- **WHEN** two interfaces display the same item state
- **THEN** both obtain its formatting from the shared rendering module

#### Scenario: The hub serves data, not display strings

- **WHEN** a client shows a priority, a status or a chat participant
- **THEN** the hub supplies the code, the status and the sender, and the shared
  rendering module supplies the word, the glyph and the colour

#### Scenario: Rendering does not depend on the service

- **WHEN** the shared rendering module is compiled
- **THEN** it imports no service package, so its presentation constants are its own
  rather than re-exported from the core

### Requirement: External adapters are isolated

Every external service or wrapped external tool SHALL be implemented as its own
dedicated adapter package, one package per tool, under `internal/adapter/<tool>`.
Internal logic SHALL reach external tools only through these adapters and SHALL
NOT shell out to or call an external tool directly. The adapters this requires
include at least git, podman, tmux, td, and openspec. Two tools SHALL NOT be wrapped
twice: a second call site for a tool that already has an adapter is a violation, not a
convenience.

Where a family of implementations exists for one job, the core SHALL depend on the
**port** — the interface naming the job — and SHALL NOT name a concrete adapter.
Choosing the implementation is the composition root's work: the core receives it and
never constructs it. This is what forbids the core from naming a tool, and it is
satisfied today by the container-runtime port (podman and Apple `container` behind
it), the coding-agent port, and the task-source port.

Where a tool has exactly one implementation and no plausible second — git and tmux —
the core MAY import its adapter directly. The abstraction earns its place by making an
implementation swappable or fakeable; requiring one where nothing can vary buys
nothing and hides the tool being used.

#### Scenario: Internal logic needs external data

- **WHEN** internal logic needs data from an external tool or API
- **THEN** it goes through that tool's adapter package rather than calling the tool
  directly

#### Scenario: One adapter per tool

- **WHEN** a new external tool is integrated
- **THEN** it gets its own `internal/adapter/<tool>` package, and no logic package
  invokes the tool outside that adapter

#### Scenario: A family is reached through its port

- **WHEN** the core needs a job done that has more than one possible implementation
  (a task source, a container runtime, a coding agent)
- **THEN** it depends on the port and is handed an implementation by the composition
  root, naming no concrete adapter itself

#### Scenario: Constructing the implementation is not the core's

- **WHEN** the set of implementations for a port is assembled
- **THEN** it is assembled where the application is composed, not inside the logic
  that consumes the port

#### Scenario: A single-implementation tool is imported directly

- **WHEN** the core needs git or tmux
- **THEN** it imports that adapter directly, because there is one implementation and
  the port would abstract nothing

#### Scenario: The core does not shell out

- **WHEN** the core needs a process probed, a repository inspected, or a quality gate
  run
- **THEN** it calls an adapter that owns that tool, rather than building a command
  itself

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

- `logic` — domain model and rules. MUST NOT import UI, rendering, or assembly
  packages, and MUST NOT call external tools/APIs. MAY import a port, and MAY import
  the adapter of a single-implementation tool (see External adapters are isolated).
  The bottom layer.
- `adapter` — wraps one external tool, API or store. MAY import `logic` for its types.
  MUST NOT import UI or rendering, and MUST NOT contain domain rules.
- `assembly` — composes adapters + logic into the app's state, and is where a port's
  implementation is chosen. MAY import `logic` and `adapter`. MUST NOT import UI or
  rendering.
- `rendering` — maps state to presentation (styles, formatting, display words,
  glyphs). MAY import `logic` types. MUST NOT import UI, adapters, assembly, or any
  service package, and holds no data logic.
- `ui` — a specific interface (TUI/GUI). MAY import `logic`, `assembly`,
  `rendering`, and `adapter` (for mutations). MUST NOT implement domain logic.
- `command` — a CLI subcommand wrapper. Same dependency freedom as `ui`; thin.
- `entrypoint` — wires a command tree and dispatches. No logic.

The list SHALL be closed: a `type:` outside it is a violation, and a test SHALL assert
that, so the vocabulary cannot drift one file at a time. A persistence package is an
`adapter` — it wraps an external store.

#### Scenario: UI contains no logic

- **WHEN** a `ui` or `command` file changes state
- **THEN** it calls `logic`/`assembly`/`adapter`, never reimplementing the rule

#### Scenario: Logic stays pure

- **WHEN** a `logic` file is compiled
- **THEN** it imports no assembly, rendering, or UI package, and calls no external
  tool except through an adapter it is permitted to import

#### Scenario: An unknown type is rejected

- **WHEN** a file declares a `type:` that is not one of the seven
- **THEN** the build fails on a test naming the file and the unknown value

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

Every non-test source file SHALL begin with a structured header with four
fields. Test files (`*_test.go`) are exempt — their subject is named by the file
they test.

- `package:` — the package, optionally `package / file` to name the file's role
- `type:` — one of: `logic`, `adapter`, `assembly`, `rendering`, `ui`,
  `command`, `entrypoint`
- `job:` — one or two lines on what this file does
- `limits:` — what this file deliberately does NOT do. Where a neighbouring package
  owns an excluded concern, the entry SHALL name it (`-> package X`), so the reader is
  sent somewhere rather than merely told no. Where the exclusion has no owner — a file
  that simply does not do I/O — a plain statement suffices.

#### Scenario: Reading a file's role

- **WHEN** a developer opens any source file
- **THEN** its header states package, type, job, and limits, and each excluded concern
  that a neighbour owns names that neighbour

#### Scenario: Header example

- **WHEN** the td adapter is opened
- **THEN** its header reads, in effect:
  `package: td` / `type: adapter (external tool)` /
  `job: wraps the td CLI, converting td JSON to issue.Task` /
  `limits: doesn't assemble issues (-> board) nor render them (-> render)`

#### Scenario: An exclusion with no owner

- **WHEN** a file's `limits:` states that it holds no state or performs no I/O, and no
  neighbour owns that concern
- **THEN** the plain statement satisfies the field, with no pointer invented for it

### Requirement: Package placement by ownership

Packages SHALL be placed by ownership to keep dependencies an acyclic graph. A
package used by exactly one owner SHALL live as a subdirectory of that owner. A
package with more than one owner — adapters, shared tools, helpers, generics —
SHALL live at `internal/<package>`. Import cycles SHALL NOT exist.

#### Scenario: Single-owner package

- **WHEN** a package is used by exactly one other package
- **THEN** it lives as a subdirectory of its owner, not at the top of `internal/`

#### Scenario: Shared package promoted

- **WHEN** a package gains a second distinct owner
- **THEN** it lives at `internal/<package>` so the shared ownership is explicit

### Requirement: The hub hosts the logic; UIs and agents are clients

Application logic SHALL be hosted by a single global hub process — the one writer of
external state for every repository it serves. All user interfaces (CLI, TUI) and all
agents SHALL be thin clients of that hub, passing the repo (project) each request
concerns, rather than mutating external state themselves. The hub SHALL keep its state
centrally, outside any repo, never per-repo on disk.

The hub SHALL be a separate program, built as its own binary, and a front-end SHALL
NOT link the hub's code into its own process. A front-end that needs a hub running
SHALL start it by executing that binary. A front-end package SHALL reach the hub only
through the exchange format and the client, and SHALL import no other part of the
core; this SHALL be enforced by a test over the import graph rather than left to
convention.

#### Scenario: A UI changes state

- **WHEN** a CLI or TUI action mutates state for a repo
- **THEN** it calls the global hub with that repo's context, which performs the
  change, rather than writing td/git/the store itself

#### Scenario: Front-end links no core code

- **WHEN** a front-end binary is built
- **THEN** it contains no hub package, no persistence driver and no adapter that only
  the hub needs — it carries the exchange format and the client, and nothing else of
  the core

#### Scenario: Starting the hub is executing it

- **WHEN** a front-end finds no hub running and needs one
- **THEN** it executes the hub binary, rather than constructing the hub in its own
  process

#### Scenario: The boundary is checked, not trusted

- **WHEN** a front-end package gains an import of a core package
- **THEN** the build fails on an import-graph test that names the offending package,
  so the violation cannot reach review

### Requirement: Domain model is a standalone package

A shared domain entity's model and its pure logic SHALL live in its own package that both the orchestrating service and every user interface depend on. This covers its data types, identity/naming, state derivation, and presentation formatting. A user interface SHALL NOT import the orchestrating service package merely to reference a domain type. The domain package SHALL NOT depend on any service package or any adapter package, so it stays testable in isolation and free of orchestration concerns.

That package SHALL be the **exchange format**: the types that cross the boundary
between the hub and its clients, together with the pure functions over them. It SHALL
import nothing beyond the standard library — no service package, no adapter, no
configuration loader — because both sides depend on it and neither may drag the other
through it. Persistence SHALL import the exchange format rather than define it: a
stored row type SHALL NOT be what clients receive, so the service never publishes its
storage layout as its API. Where an internal type cannot cross the wire — one holding
a function or an interface, for instance — the exchange format SHALL carry the
resolved value instead, and the internal type SHALL stay behind.

#### Scenario: UI references a domain type

- **WHEN** a CLI or TUI names a domain type (e.g. an agent view) or calls its pure
  logic (e.g. computing a container name or formatting its state)
- **THEN** it imports the domain package rather than the orchestrating service
  package

#### Scenario: Domain package has no service or adapter imports

- **WHEN** the domain package is compiled
- **THEN** it imports no orchestrating-service package and no adapter package
  (store, podman, tmux, …)

#### Scenario: Service and UI share one model

- **WHEN** the service and a UI both handle the same domain entity
- **THEN** both obtain its type and pure logic from the same domain package, not
  from each other

#### Scenario: The exchange format imports nothing

- **WHEN** the exchange package is compiled
- **THEN** it imports only the standard library, and a test asserts that, so neither
  side can acquire a dependency through it

#### Scenario: Storage rows are not the published API

- **WHEN** a client receives a task, a PR or a project
- **THEN** it receives an exchange type that the persistence layer also uses, rather
  than a row type the persistence layer defines

## Source layout

The layered structure that realizes the rules above:

- `internal/api/` — **data.** The exchange format: every type that crosses the
  wire, plus the pure derivations over them. It imports nothing else internal, so
  both sides of the socket depend on it without depending on each other.
- `internal/hub/` — **the core.** The only process with domain logic, split by
  concern: `workflow` (the task/PR lifecycle), `task`, `comments`, `project`,
  `registry`, `chat`, `agent` (pod lifecycle), `agentchan` (the per-agent
  socket), `commands` (the role-filtered surface), `repo` (git/PR mechanics and
  the submit gate), `server` (HTTP over the socket) and `store` (SQLite).
- `internal/client/` — **transport.** The wire client every front-end reaches the
  hub through; it holds no domain logic of its own.
- `internal/adapter/` — **adapters.** The only code that touches the outside
  world: `git`, `container`, `tmux`, `agent` (the coding-agent backend, with
  `agent/claude`), `tasks` (the trackers) and `herdr`.
- `internal/ui/` — **front-ends.** `cli` and `tui` are interchangeable thin
  layers over the client; `theme` is the UI-neutral rendering both share; `attach`
  composes what an interactive attach needs. Nothing here imports `internal/hub`,
  and a test walks the import graph to keep it that way.
- `internal/config/`, `internal/container/`, `internal/update/`,
  `internal/tools/` (`paths`, `debug`) — shared support: the project config, the
  container-runtime port and image build, the release check, and the filesystem
  and diagnostic helpers.
- `internal/brokkr/` (`codemap`, `lint`) — the toolbelt's logic, kept out of the
  product: the code map and the linters.
- `cmd/sindri/` — the host CLI and TUI launcher. `cmd/sindri-hub/` — the hub
  binary. `cmd/sindri-worker/` — the thin browser an agent drives inside its pod.
  `cmd/brokkr/` — the separate dev-tooling binary.

