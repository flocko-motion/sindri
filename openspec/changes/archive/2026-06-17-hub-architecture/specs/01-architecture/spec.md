# Architecture — delta

## MODIFIED Requirements

### Requirement: External adapters are isolated

Every external service or wrapped external tool SHALL be implemented as its own
dedicated adapter package, one package per tool, under `internal/adapter/<tool>`.
Internal logic SHALL reach external tools only through these adapters and SHALL
NOT shell out to or call an external tool directly. The adapters this requires
include at least git, podman, tmux, td, and openspec.

#### Scenario: Internal logic needs external data

- **WHEN** internal logic needs data from an external tool or API
- **THEN** it goes through that tool's adapter package rather than calling the tool
  directly

#### Scenario: One adapter per tool

- **WHEN** a new external tool is integrated
- **THEN** it gets its own `internal/adapter/<tool>` package, and no logic package
  invokes the tool outside that adapter

## ADDED Requirements

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

Application logic SHALL be hosted by the hub process, which is the single writer
of external state. All user interfaces (CLI, TUI) and all agents SHALL be thin
clients of the hub rather than mutating external state themselves.

#### Scenario: A UI changes state

- **WHEN** a CLI or TUI action mutates state
- **THEN** it calls the hub, which performs the change, rather than writing td/git/
  the store itself
