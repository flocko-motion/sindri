## ADDED Requirements

### Requirement: The hub serves two command surfaces, and they stay apart

The hub SHALL serve exactly two command surfaces: the agent surface, reached by
`sindri-worker` over an agent's own channel, and the front-end surface, reached by the host
CLI and the TUI over the control socket. Each SHALL live in its own directory under
`internal/hub/api/`, and neither SHALL import the other. Whatever both need SHALL sit beside
them in `internal/hub/api/serve/` rather than in one of them.

The agent surface SHALL answer three endpoints — the caller's verb list, its directive, and
one execute — and every verb SHALL be reached through that one execute. The front-end
surface SHALL answer one route per operation.

#### Scenario: The surfaces do not reach into each other

- **WHEN** the tree is walked for imports
- **THEN** no file under `internal/hub/api/agents` imports `internal/hub/api/frontend`, and
  no file under `internal/hub/api/frontend` imports `internal/hub/api/agents`

#### Scenario: A new agent verb needs no route

- **WHEN** a verb is added to the agent surface
- **THEN** it is reachable through the existing execute endpoint, and the front-end's route
  table is unchanged

#### Scenario: The grouping directory declares nothing

- **WHEN** `internal/hub/api/` is read
- **THEN** it holds only subdirectories, so no package is named `api` beside the exchange
  package `internal/api`

### Requirement: A verb is defined once, as data

Every verb the agent surface offers SHALL be declared exactly once, in one catalogue package,
as a value carrying its name, the one line a state's offer list prints, its full usage, the
roles it belongs to, and any caller-tailored help. A verb definition SHALL hold no
implementation: the catalogue is data, and binding a verb to what it runs SHALL happen in one
table outside it.

The catalogue's help SHALL be the only help the agent reads, whether it arrives in a state's
offer list, in the verb surface, or from asking the verb for its own help.

#### Scenario: One help text reaches the agent

- **WHEN** an agent reads a verb's help in its directive, in its command surface, and by
  asking the verb itself
- **THEN** all three come from that verb's single definition and agree word for word

#### Scenario: The catalogue names no writer

- **WHEN** the catalogue package's imports are read
- **THEN** it reaches nothing that can write a row, steer a session, read a clock or start a
  context, so a flow declaration may import it

#### Scenario: A verb declared and never bound is caught

- **WHEN** a verb is in the catalogue with no entry in the binding table
- **THEN** the build or a guard reports it, rather than the agent meeting a verb that does
  nothing

### Requirement: One operation has one core, however it is reached

Where both surfaces drive the same operation, that operation SHALL be implemented once as an
exported function in the subject package that owns it — a pull request's in the pull-request
package, a task's in the task package, a message's in the mailbox. A verb adapter SHALL parse
argv and render prose for the agent; a route adapter SHALL decode and return JSON. Neither
adapter SHALL hold a rule the other needs.

Where an operation reads differently by caller, the core SHALL take who is asking as a
parameter rather than being written twice.

#### Scenario: Approving from either side

- **WHEN** an agent runs `approve` and a user approves the same pull request from the TUI
- **THEN** both reach one core function, and the record each leaves differs only in the
  caller it names

#### Scenario: An adapter carries no rule

- **WHEN** a verb adapter or a route handler is read
- **THEN** it parses its input, calls one exported core, and renders the result

#### Scenario: A guard rejects a second implementation

- **WHEN** a verb adapter or a route handler is written that decides an operation for itself
  rather than calling its core
- **THEN** the architecture guard names the file and fails

### Requirement: A verb name means one thing

A name SHALL carry the same operation on both surfaces, or belong to only one of them. A name
that reads as a write to an agent SHALL NOT read as a read to a front-end.

#### Scenario: The activity log is read under its own name

- **WHEN** a front-end fetches the activity log
- **THEN** it asks for the activity, and `log` stays the agent's verb for recording a note
