## ADDED Requirements

### Requirement: Domain model is a standalone package

A shared domain entity's model and its pure logic SHALL live in its own package that both the orchestrating service and every user interface depend on. This covers its data types, identity/naming, state derivation, and presentation formatting. A user interface SHALL NOT import the orchestrating service package merely to reference a domain type. The domain package SHALL NOT depend on any service package or any adapter package, so it stays testable in isolation and free of orchestration concerns.

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
