# Architecture — delta

## MODIFIED Requirements

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
