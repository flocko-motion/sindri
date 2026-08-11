# Architecture — delta

## MODIFIED Requirements

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
