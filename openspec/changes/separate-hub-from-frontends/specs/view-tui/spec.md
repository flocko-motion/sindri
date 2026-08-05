# View: TUI dashboard — delta

## MODIFIED Requirements

### Requirement: The TUI is a hub client

The TUI SHALL get all data from the single global hub (`/state` + `/events`) and
perform all mutations through it, holding no domain logic of its own. When no hub is
running it SHALL auto-start a background hub rather than refusing.

It SHALL reach the hub only through the client and the exchange format, importing no
other part of the core — no hub package, no persistence package, no adapter the hub
owns. Where it needs something the hub knows, it SHALL ask the hub rather than
computing it in its own process: whether an optional external tool is installed is
the hub's answer to give, because the hub is the process that runs it.

#### Scenario: No hub yet

- **WHEN** the TUI starts and no hub is running
- **THEN** it starts a background hub, then connects

#### Scenario: The TUI imports no core package

- **WHEN** the TUI package is compiled
- **THEN** it imports the client and the exchange format, and no hub or persistence
  package

#### Scenario: Tool availability is the hub's answer

- **WHEN** the TUI reports that an optional external tool is missing
- **THEN** the finding came from the hub, which is the process that would invoke the
  tool, rather than from the TUI probing its own environment
