# agent-runtime — delta

## ADDED Requirements

### Requirement: The machine's memory headroom is answered by the runtime

The container runtime SHALL report the fleet's memory: what running agents cost the machine against
the ceiling they draw from, and what the used figure counts. The backend SHALL answer it, for the
reason it answers the default limit — the same two numbers mean different things per backend.
Containers sharing the host kernel take memory as they use it and the machine may be overcommitted;
a per-container micro-VM reserves its whole limit up front, so an agent whose reservation does not
fit cannot start however little the fleet is using; and a runtime that runs its containers inside a
VM is bounded by that VM rather than by the machine hosting it. Neither the hub nor a front-end
SHALL read the host's memory for this figure.

From that reading the hub SHALL derive the fleet's headroom — the memory left, and how many further
agents of the default size fit in it — and carry it on the board, so that whether another agent fits
is answerable without arithmetic. The count SHALL name the agent size it counted in. An
overcommitted machine SHALL report no free memory rather than a negative amount.

A reading the runtime cannot give SHALL leave the headroom unknown, and an unknown headroom SHALL be
rendered as nothing rather than as a machine with nothing free. The reading SHALL be taken on the
hub's own cadence and reported from the last one taken, so that serving the board costs no
measurement.

Wherever memory is reported to the user, the fleet's headroom SHALL be reported in the same terms in
both front-ends.

#### Scenario: Headroom is counted in agents

- **WHEN** the machine has memory to spare
- **THEN** the board reports what is free and how many more agents of the default size fit there,
  naming that size

#### Scenario: Reserved and in use are different questions

- **WHEN** the backend gives each agent its own micro-VM
- **THEN** the figure is what the fleet has reserved, since a reservation is held whether it is used
  or not

#### Scenario: The runtime cannot say

- **WHEN** the runtime gives no reading, or has not yet been asked
- **THEN** the headroom is unknown, and nothing is drawn in its place

#### Scenario: An overcommitted machine

- **WHEN** the fleet's memory exceeds the ceiling it draws from
- **THEN** the free memory reads as none and no further agent is reported to fit

#### Scenario: The user asks what agents are using

- **WHEN** the user asks for agent memory from the CLI
- **THEN** the fleet's headroom is reported alongside the per-agent usage
